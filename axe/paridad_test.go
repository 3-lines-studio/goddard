package axe

import (
	"encoding/json"
	"strconv"
	"testing"
)

const (
	probeSchema = `{"type":"object","properties":{"req":{"type":"string"},"s":{"type":"string"},"i":{"type":"integer"},"n":{"type":"number"},"b":{"type":"boolean"},"arr":{"type":"array","items":{"type":"integer"}},"o":{"type":"object","properties":{"x":{"type":"integer"}}},"u":{"type":["string","null"]}},"required":["req"]}`
	probeTool   = `{"type":"object","properties":{"n":{"type":"integer"}},"required":["n"]}`
)

type probeArgs struct {
	N int64 `json:"n"`
}

func newProbe() Tool {
	return NewTool("probe", "d", probeTool, func(args probeArgs) string {
		return "n=" + strconv.FormatInt(args.N, 10)
	})
}

func marshal(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "marshal error: " + err.Error()
	}
	return string(data)
}

func coerceDump(schema, raw string) string {
	var schemaValue any
	json.Unmarshal([]byte(schema), &schemaValue)
	args := parseArgs(raw)
	if args != nil {
		coerceArgs(&args, schemaValue)
	}
	return marshal(args)
}

func paridadCases() map[string]func() string {
	return map[string]func() string{
		"message_default": func() string { return marshal(Message{}) },
		"message_user":    func() string { return marshal(Message{Role: "user", Content: "hola"}) },
		"message_assistant_tools": func() string {
			return marshal(Message{
				Role:      "assistant",
				Content:   "voy",
				ToolCalls: []ToolCall{{ID: "c1", Name: "bash", Arguments: `{"command":"ls"}`}},
				Reasoning: "penso",
			})
		},
		"message_tool_image": func() string {
			return marshal(Message{
				Role:       "tool",
				Content:    "ok",
				ToolCallID: "c1",
				Images:     []Image{{Path: "/a.png"}},
			})
		},
		"image_empty": func() string { return marshal(Image{}) },
		"image_full":  func() string { return marshal(Image{Path: "/a.png", URL: "https://x/y.png"}) },
		"tool_call":   func() string { return marshal(ToolCall{ID: "c1", Name: "read", Arguments: "{}"}) },
		"deser_old_message": func() string {
			var message Message
			if err := json.Unmarshal([]byte(`{"Role":"user","Content":"hi"}`), &message); err != nil {
				return "unmarshal error: " + err.Error()
			}
			return marshal(message)
		},
		"deser_tool_call": func() string {
			var call ToolCall
			json.Unmarshal([]byte(`{"ID":"c1","Name":"read","Arguments":"{}"}`), &call)
			return marshal(call)
		},

		"coerce_bash_timeout_string": func() string { return coerceDump(bashSchema, `{"command":"ls","timeout":"30"}`) },
		"coerce_bash_timeout_float":  func() string { return coerceDump(bashSchema, `{"command":"ls","timeout":30.0}`) },
		"coerce_bash_command_int":    func() string { return coerceDump(bashSchema, `{"command":123,"timeout":5}`) },
		"coerce_bash_command_bool":   func() string { return coerceDump(bashSchema, `{"command":true}`) },
		"coerce_bash_missing":        func() string { return coerceDump(bashSchema, `{ `) },
		"coerce_bash_empty_raw":      func() string { return coerceDump(bashSchema, ``) },
		"coerce_bash_timeout_null":   func() string { return coerceDump(bashSchema, `{"command":"ls","timeout":null}`) },
		"coerce_bash_command_null":   func() string { return coerceDump(bashSchema, `{"command":null}`) },
		"coerce_bash_extra_key":      func() string { return coerceDump(bashSchema, `{"command":"ls","zzz":1}`) },
		"coerce_edit_ok": func() string {
			return coerceDump(editSchema, `{"path":"/a","edits":[{"oldText":"x","newText":"y"}]}`)
		},
		"coerce_edit_extra_key": func() string {
			return coerceDump(editSchema, `{"path":"/a","edits":[],"extra":"z"}`)
		},
		"coerce_edit_array_scalar": func() string {
			return coerceDump(editSchema, `{"path":"/a","edits":["x"]}`)
		},
		"coerce_array_of_arrays": func() string {
			return coerceDump(`{"type":"array","items":{"type":"array","items":{"type":"integer"}}}`, `[["1",2]]`)
		},

		"probe_s_from_int":          func() string { return coerceDump(probeSchema, `{"req":"x","s":42}`) },
		"probe_s_from_bool":         func() string { return coerceDump(probeSchema, `{"req":"x","s":true}`) },
		"probe_s_null":              func() string { return coerceDump(probeSchema, `{"req":"x","s":null}`) },
		"probe_s_from_array":        func() string { return coerceDump(probeSchema, `{"req":"x","s":["a"]}`) },
		"probe_req_from_int":        func() string { return coerceDump(probeSchema, `{"req":123}`) },
		"probe_req_from_float":      func() string { return coerceDump(probeSchema, `{"req":1.5}`) },
		"probe_req_null":            func() string { return coerceDump(probeSchema, `{"req":null}`) },
		"probe_i_from_float_string": func() string { return coerceDump(probeSchema, `{"req":"x","i":"3.5"}`) },
		"probe_i_from_sci_string":   func() string { return coerceDump(probeSchema, `{"req":"x","i":"1e3"}`) },
		"probe_i_from_bool":         func() string { return coerceDump(probeSchema, `{"req":"x","i":true}`) },
		"probe_i_from_float":        func() string { return coerceDump(probeSchema, `{"req":"x","i":1.9}`) },
		"probe_i_null":              func() string { return coerceDump(probeSchema, `{"req":"x","i":null}`) },
		"probe_n_from_string":       func() string { return coerceDump(probeSchema, `{"req":"x","n":"3.5"}`) },
		"probe_n_from_int":          func() string { return coerceDump(probeSchema, `{"req":"x","n":7}`) },
		"probe_n_from_bool":         func() string { return coerceDump(probeSchema, `{"req":"x","n":true}`) },
		"probe_n_null":              func() string { return coerceDump(probeSchema, `{"req":"x","n":null}`) },
		"probe_b_from_string":       func() string { return coerceDump(probeSchema, `{"req":"x","b":"true"}`) },
		"probe_b_from_one":          func() string { return coerceDump(probeSchema, `{"req":"x","b":1}`) },
		"probe_b_from_zero":         func() string { return coerceDump(probeSchema, `{"req":"x","b":0}`) },
		"probe_b_from_other_string": func() string { return coerceDump(probeSchema, `{"req":"x","b":"yes"}`) },
		"probe_b_from_float":        func() string { return coerceDump(probeSchema, `{"req":"x","b":2.0}`) },
		"probe_b_null":              func() string { return coerceDump(probeSchema, `{"req":"x","b":null}`) },
		"probe_arr_strings":         func() string { return coerceDump(probeSchema, `{"req":"x","arr":["1","2"]}`) },
		"probe_arr_floats":          func() string { return coerceDump(probeSchema, `{"req":"x","arr":[1.7,2]}`) },
		"probe_arr_not_array":       func() string { return coerceDump(probeSchema, `{"req":"x","arr":"nope"}`) },
		"probe_obj_nested":          func() string { return coerceDump(probeSchema, `{"req":"x","o":{"x":"5"}}`) },
		"probe_obj_extra":           func() string { return coerceDump(probeSchema, `{"req":"x","o":{"x":"5","z":2}}`) },
		"probe_union_null":          func() string { return coerceDump(probeSchema, `{"req":"x","u":null}`) },
		"probe_only_required":       func() string { return coerceDump(probeSchema, `{"req":"x"}`) },
		"probe_extra_root_key":      func() string { return coerceDump(probeSchema, `{"req":"x","zzz":1}`) },

		"tool_run_ok": func() string { return newProbe().Run(`{"n":"7"}`, nil).Text },
	}
}

func toolParams(t *testing.T) map[string]string {
	t.Helper()
	params := map[string]string{}
	for _, tool := range BuildTools("/tmp") {
		params["tool_params_"+tool.Name] = marshal(tool.Parameters)
	}
	return params
}

func TestParidadConRust(t *testing.T) {
	expected := readTestdata(t, "testdata/paridad-rust.txt")
	cases := paridadCases()
	pendientes := map[string]bool{
		"system_prompt":      true,
		"tool_params_search": true,
		"tool_params_fetch":  true,
	}
	for name, want := range toolParams(t) {
		cases[name] = func() string { return want }
	}
	cubiertosEnOtroTest := map[string]bool{
		"tool_run_empty":     true,
		"tool_run_blank":     true,
		"tool_run_missing":   true,
		"tool_run_bad_json":  true,
		"tool_run_null_root": true,
	}
	for name := range expected {
		_, covered := cases[name]
		if !covered && !pendientes[name] && !cubiertosEnOtroTest[name] {
			t.Errorf("el dump trae el caso %q y nadie lo cubre", name)
		}
	}
	for name, want := range expected {
		check, covered := cases[name]
		if !covered {
			continue
		}
		if got := check(); got != want {
			t.Errorf("%s:\n got:  %s\n want: %s", name, got, want)
		}
	}
}

func TestNewToolRechazaArgumentosInvalidos(t *testing.T) {
	probe := newProbe()
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			"vacio",
			"",
			"error: invalid arguments for probe: missing field `n`\nReceived: {}",
		},
		{
			"espacios",
			"   ",
			"error: invalid arguments for probe: missing field `n`\nReceived: {}",
		},
		{
			"sin n",
			"{}",
			"error: invalid arguments for probe: missing field `n`\nReceived: {}",
		},
		{
			"json roto",
			"{ nope",
			"error: invalid arguments for probe: invalid type: expected an object\nReceived: { nope",
		},
		{
			"raiz nula",
			"null",
			"error: invalid arguments for probe: invalid type: expected an object\nReceived: null",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := probe.Run(test.raw, nil).Text; got != test.want {
				t.Errorf("\n got:  %q\n want: %q", got, test.want)
			}
		})
	}

	if got := newProbe().Run(`{"n":7}`, nil).Text; got != "n=7" {
		t.Errorf("n entero: %q", got)
	}
}
