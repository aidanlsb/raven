package cli

var vaultPathCmd = newCanonicalLeafCommand("vault_path", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderVaultList,
})
