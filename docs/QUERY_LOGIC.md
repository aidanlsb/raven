# Query Logic

* There are two types of queries: object queries and trait queries
* Queries can contain sub-queries (more detail below)
* Every query, including sub-queries, can only return an object/trait of a single type/name
* Each query type (object and trait) has a valid set of optional predicates that can be combined arbitrarily using standard boolean logic (AND, OR, NOT)
* Each predicate "refers to" or is "based on" a specific thing. Examples below

## Query Types and Valid Predicates

### Object Query Predicates
* Field-based:
    * Field equals a certain value
    * Field is not a certain value
    * Field is missing
    * Field exists
    * The above can all be combined with boolean logic, e.g. we can express "this field exists and is not a certain value"
* Trait-based:
    * Predicate is a valid trait query (below)
* Parent
    * Predicate is a valid object query itself, filters objects whose parent matches the subquery
* Child
    * Predicate is a valid object query itself, filters objects that have at least one child matching the subquery

### Trait Query Predicates
* Value-based (basically the same as Object field predicates):
    * Trait's value equals a certain value
    * Trait's value is not a certain value
* Source (if omitted all sources included) 
    * Inline
    * Frontmatter
* Object (<-- we need a new name for this that is less ambiguous with object queries directly)
    * The object the trait is associated with <-- we also need to discuss this since it's vague: is it the trait's direct parent, or do we include all ancestors? E.g. a trait in the content within a meeting within a daily note


Additional topic to discuss:
* Temporal queries. I think the audit log enables them for agentic queries. How do we want to handle them directly in the query system (if at all)?

