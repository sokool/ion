package ion

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/alecthomas/jsonschema"
)

type Tool struct {
	Name string
	// Execute is called when LLM decides to call it in LLM API
	// bool = true make llm call again with returned string
	Execute func(JSON) (string, bool)
	// Schemas represent function input json schema objects
	Schemas []Meta
}

// NewToolMD parses the provided markdown string to extract function definitions and
// schemas, then returns a Tool configured with the given function handler.
// Parameters:
//   - markdown: string containing markdown documentation with function definitions
//   - fn: Function handler that will be called when this tool is invoked
func NewToolMD(markdown string, fn Function) (Tool, error) {
	var err error
	t := Tool{Execute: fn}
	if t.Schemas, err = t.parseMD(markdown); err != nil {
		return t, err
	}
	return t, nil
}

func NewTool[T any](name, desc string, fn func(T) (string, bool)) (Tool, error) {
	var v T
	var n string

	b, err := json.MarshalIndent((&jsonschema.Reflector{}).Reflect(v), "", "  ")
	if err != nil {
		return Tool{}, err
	}
	j := JSON(b)
	for _, n = range j.Each("definitions") {
		break
	}
	if n == "" {
		return Tool{}, ErrTool.New("invalid %s definition", v)
	}

	return Tool{
		Name: name,
		Execute: func(j JSON) (string, bool) {
			var t T
			if err := j.To(&t); err != nil {
				log_.Errorf("Tool %s could not decode data to a %T type", name, t)
				return "", false
			}
			return fn(t)
		},
		Schemas: []Meta{
			{
				"function": Meta{
					"description": desc,
					"name":        name,
					"parameters":  j.Select("definitions").Select(n),
				},
				"type": "function",
			},
		},
	}, nil
}

func MustLLMTool[T any](name, desc string, fn func(T) (string, bool)) Tool {
	t, err := NewTool(name, desc, fn)
	if err != nil {
		panic(err)
	}
	return t
}

func (t Tool) HasName(n string) bool {
	if t.Name == n {
		return true
	}
	for _, s := range t.Schemas {
		if s.JSON("function").Text("name") == n {
			return true
		}
	}
	return false
}

func (t Tool) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`{"Function":"%s", "Schemas": "%s"}`, reflect.TypeOf(t.Execute), t.Schemas)), nil
}

func (t *Tool) UnmarshalJSON(b []byte) error {
	// todo
	return nil
}

func (t Tool) parseMD(s string) ([]Meta, error) {
	var scm []Meta
	scn := bufio.NewScanner(strings.NewReader(s))
	fni := 0
	var fns []string
	for scn.Scan() {
		ln := scn.Text()
		if i := strings.Index(ln, "## Function:"); i >= 0 {
			fns, fni = append(fns, ""), fni+1
		}
		if len(fns) == fni && fni != 0 {
			fns[fni-1] += ln + "\n"
		}
	}
	if err := scn.Err(); err != nil {
		return nil, err
	}
	var errs []error
	for i := range fns {
		if p, err := t.toJSONSchema(fns[i]); err != nil {
			errs = append(errs, err)
		} else {
			scm = append(scm, p)
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return scm, nil
}

func (t Tool) toJSONSchema(s string) (Meta, error) {
	var (
		re                             = regexp.MustCompile(`^(\w+)\s+(\S+)\s*-\s*(.*)$`)
		lns                            = strings.Split(s, "\n")
		title, intro, param, typ, desc string
		props                          = make(Meta)
		reqs                           []string
		buildParam                     = func(typ, desc string) Meta {
			desc = strings.TrimSpace(desc)
			if strings.HasPrefix(typ, "array[") && strings.HasSuffix(typ, "]") {
				return Meta{"type": "array", "description": desc, "items": Meta{"type": typ[6 : len(typ)-1]}}
			}
			return Meta{"type": typ, "description": desc}
		}
	)

	for i, s := range lns {
		s = strings.TrimSpace(s)
		isLast := i == len(lns)-1

		if m := re.FindStringSubmatch(s); len(m) == 4 || isLast {
			if param != "" || isLast {
				props[param] = buildParam(typ, desc)
				reqs = append(reqs, param)
			}
			if isLast {
				break
			}
			param, typ, desc = m[1], m[2], m[3]
			continue
		}

		switch {
		case param != "":
			desc += "\n" + s
		case title != "":
			intro += s + "\n"
		case strings.HasPrefix(s, "## Function:"):
			title = strings.TrimSpace(strings.TrimPrefix(s, "## Function:"))
		}
	}

	return Meta{
		"type": "function",
		"function": Meta{
			"name":        title,
			"description": strings.TrimSpace(intro),
			"parameters": Meta{
				"type":                 "object",
				"properties":           props,
				"required":             reqs,
				"additionalProperties": false,
			},
			"strict": true,
		},
	}, nil
}

var ErrTool = ErrCompletion.New("tool")

type Function func(JSON) (string, bool)
