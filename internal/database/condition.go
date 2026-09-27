package database

import "strings"

// Condition builds a SQL WHERE clause fragment incrementally, mirroring the
// Python Condition class in database.py. Call SQL() to get the clause and
// Args() to get the corresponding positional arguments.
type Condition struct {
	sql    strings.Builder
	args   []any
	limit  int
	offset int
	order  string
	desc   bool
}

func (c *Condition) hasSQL() bool { return c.sql.Len() > 0 }

func (c *Condition) append(op, fragment string, arg any) *Condition {
	if c.hasSQL() {
		c.sql.WriteString(op)
	}
	c.sql.WriteString(fragment)
	c.args = append(c.args, arg)
	return c
}

func (c *Condition) OrEqual(column, value string, caseSensitive bool) *Condition {
	col, val := normalize(column, value, caseSensitive)
	return c.append(" OR ", col+"=?", val)
}

func (c *Condition) AndEqual(column, value string, caseSensitive bool) *Condition {
	col, val := normalize(column, value, caseSensitive)
	return c.append(" AND ", col+"=?", val)
}

func (c *Condition) OrLike(column, pattern string, caseSensitive bool) *Condition {
	col, pat := normalize(column, pattern, caseSensitive)
	return c.append(" OR ", col+" LIKE ?", pat)
}

func (c *Condition) AndLike(column, pattern string, caseSensitive bool) *Condition {
	col, pat := normalize(column, pattern, caseSensitive)
	return c.append(" AND ", col+" LIKE ?", pat)
}

func (c *Condition) OrSubCondition(sub *Condition) *Condition {
	c.args = append(c.args, sub.args...)
	op := " OR "
	if !c.hasSQL() {
		op = ""
	}
	c.sql.WriteString(op + "(" + sub.sqlFragment() + ")")
	return c
}

func (c *Condition) AndSubCondition(sub *Condition) *Condition {
	c.args = append(c.args, sub.args...)
	op := " AND "
	if !c.hasSQL() {
		op = ""
	}
	c.sql.WriteString(op + "(" + sub.sqlFragment() + ")")
	return c
}

func (c *Condition) OrNotSubCondition(sub *Condition) *Condition {
	c.args = append(c.args, sub.args...)
	op := " OR "
	if !c.hasSQL() {
		op = ""
	}
	c.sql.WriteString(op + "NOT (" + sub.sqlFragment() + ")")
	return c
}

func (c *Condition) AndNotSubCondition(sub *Condition) *Condition {
	c.args = append(c.args, sub.args...)
	op := " AND "
	if !c.hasSQL() {
		op = ""
	}
	c.sql.WriteString(op + "NOT (" + sub.sqlFragment() + ")")
	return c
}

func (c *Condition) Limit(n int) *Condition  { c.limit = n; return c }
func (c *Condition) Offset(n int) *Condition { c.offset = n; return c }
func (c *Condition) OrderBy(col string, desc bool) *Condition {
	c.order = col
	c.desc = desc
	return c
}

// sqlFragment returns the raw WHERE expression without ORDER BY / LIMIT / OFFSET.
// Used when embedding this condition inside another (sub-condition).
func (c *Condition) sqlFragment() string {
	if c.sql.Len() == 0 {
		return "1"
	}
	return c.sql.String()
}

// SQL returns the full WHERE clause including ORDER BY / LIMIT / OFFSET clauses.
func (c *Condition) SQL() string {
	s := c.sqlFragment()
	if c.order != "" {
		s += " ORDER BY " + c.order
		if c.desc {
			s += " DESC"
		}
	}
	if c.limit > 0 {
		s += " LIMIT ?"
	}
	if c.offset > 0 {
		s += " OFFSET ?"
	}
	return s
}

// Args returns the positional arguments matching the placeholders in SQL().
func (c *Condition) Args() []any {
	out := make([]any, len(c.args))
	copy(out, c.args)
	if c.limit > 0 {
		out = append(out, c.limit)
	}
	if c.offset > 0 {
		out = append(out, c.offset)
	}
	return out
}

func normalize(column, value string, caseSensitive bool) (string, string) {
	if caseSensitive {
		return column, value
	}
	return "LOWER(" + column + ")", strings.ToLower(value)
}
