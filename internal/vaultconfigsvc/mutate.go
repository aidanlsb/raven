package vaultconfigsvc

import (
	"bytes"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func mutate(rt *vaultruntime.Runtime, apply func(*config.VaultConfig) error) (*MutationResult, error) {
	cfg, exists, configPath, err := load(rt)
	if err != nil {
		return nil, err
	}
	before, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to save vault config", err)
	}
	if err := apply(cfg); err != nil {
		return nil, err
	}
	after, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to save vault config", err)
	}
	changed := !bytes.Equal(before, after)
	if changed {
		if err := save(rt, cfg); err != nil {
			return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to save vault config", err)
		}
	}
	return &MutationResult{
		ConfigPath: configPath,
		Created:    !exists && changed,
		Changed:    changed,
	}, nil
}

func mutateSection[T any](
	rt *vaultruntime.Runtime,
	current func(*config.VaultConfig) *T,
	set func(*config.VaultConfig, *T),
	compact func(*T) *T,
	apply func(*T) error,
) (*MutationResult, error) {
	return mutate(rt, func(cfg *config.VaultConfig) error {
		before := current(cfg)
		next := clonePtr(before)
		if next == nil {
			next = new(T)
		}
		if err := apply(next); err != nil {
			return err
		}
		next = compact(next)
		if sectionYAMLEqual(before, next) {
			return nil
		}
		set(cfg, next)
		return nil
	})
}

func clonePtr[T any](v *T) *T {
	if v == nil {
		return nil
	}
	cloned := *v
	return &cloned
}

func sectionYAMLEqual[T any](a, b *T) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	left, err := yaml.Marshal(a)
	if err != nil {
		return false
	}
	right, err := yaml.Marshal(b)
	if err != nil {
		return false
	}
	return bytes.Equal(left, right)
}

type listAccessor struct {
	field        func(*config.VaultConfig) *[]string
	normalize    func(string) (string, error)
	normalizeAll func([]string) []string
	sort         bool
	notFound     func(string) error
}

type listChange struct {
	MutationResult
	Exists bool
	Item   string
	Values []string
}

func (a listAccessor) current(cfg *config.VaultConfig) []string {
	return a.normalizeAll(*a.field(cfg))
}

func (a listAccessor) list(rt *vaultruntime.Runtime) (configPath string, exists bool, values []string, err error) {
	cfg, exists, configPath, err := load(rt)
	if err != nil {
		return "", false, nil, err
	}
	return configPath, exists, a.current(cfg), nil
}

func (a listAccessor) add(rt *vaultruntime.Runtime, raw string) (*listChange, error) {
	normalized, err := a.normalize(raw)
	if err != nil {
		return nil, err
	}
	mutation, err := mutate(rt, func(cfg *config.VaultConfig) error {
		values := a.current(cfg)
		for _, existing := range values {
			if existing == normalized {
				return nil
			}
		}
		values = append(values, normalized)
		if a.sort {
			sort.Strings(values)
		}
		*a.field(cfg) = values
		return nil
	})
	if err != nil {
		return nil, err
	}
	return a.result(rt, mutation, normalized)
}

func (a listAccessor) remove(rt *vaultruntime.Runtime, raw string) (*listChange, error) {
	normalized, err := a.normalize(raw)
	if err != nil {
		return nil, err
	}
	mutation, err := mutate(rt, func(cfg *config.VaultConfig) error {
		values := a.current(cfg)
		next := make([]string, 0, len(values))
		found := false
		for _, existing := range values {
			if existing == normalized {
				found = true
				continue
			}
			next = append(next, existing)
		}
		if !found {
			return a.notFound(normalized)
		}
		*a.field(cfg) = next
		return nil
	})
	if err != nil {
		return nil, err
	}
	return a.result(rt, mutation, normalized)
}

func (a listAccessor) result(rt *vaultruntime.Runtime, mutation *MutationResult, item string) (*listChange, error) {
	cfg, exists, _, err := load(rt)
	if err != nil {
		return nil, err
	}
	return &listChange{
		MutationResult: *mutation,
		Exists:         exists,
		Item:           item,
		Values:         a.current(cfg),
	}, nil
}

var protectedPrefixes = listAccessor{
	field:        func(cfg *config.VaultConfig) *[]string { return &cfg.ProtectedPrefixes },
	normalize:    normalizeProtectedPrefix,
	normalizeAll: normalizedProtectedPrefixes,
	sort:         true,
	notFound: func(normalized string) error {
		return svcerr.New(codes.ErrPrefixNotFound, fmt.Sprintf("protected prefix '%s' not found", normalized)).
			WithSuggestion("Run 'rvn vault config protected-prefixes list' to see configured prefixes")
	},
}

var excludePatterns = listAccessor{
	field:        func(cfg *config.VaultConfig) *[]string { return &cfg.Exclude },
	normalize:    normalizeExcludePattern,
	normalizeAll: normalizedExcludePatterns,
	notFound: func(normalized string) error {
		return svcerr.New(codes.ErrPrefixNotFound, fmt.Sprintf("exclude pattern %q not found", normalized)).
			WithSuggestion("Run 'rvn vault config exclude list' to see configured patterns")
	},
}

func load(rt *vaultruntime.Runtime) (*config.VaultConfig, bool, string, error) {
	if err := vaultruntime.Require(rt); err != nil {
		return nil, false, "", svcerr.Wrap(codes.ErrInvalidInput, "vault path is required", err).WithSuggestion("Resolve a vault before invoking the command")
	}
	if rt.VaultCfg == nil {
		if err := rt.ReloadConfig(); err != nil {
			return nil, rt.VaultConfigExists, rt.VaultConfigPath, svcerr.Wrap(codes.ErrConfigInvalid, "failed to load vault config", err).WithSuggestion("Fix raven.yaml and try again")
		}
	}
	return rt.VaultCfg, rt.VaultConfigExists, rt.VaultConfigPath, nil
}

func save(rt *vaultruntime.Runtime, cfg *config.VaultConfig) error {
	err := config.SaveVaultConfig(rt.VaultPath, cfg)
	reloadErr := rt.ReloadConfig()
	if err != nil {
		return err
	}
	return reloadErr
}
