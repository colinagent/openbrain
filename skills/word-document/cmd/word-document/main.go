package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/colinagent/openbrain/skills/word-document/internal/docx"
)

type commandError struct {
	OK      bool   `json:"ok"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func fail(code string, err error) {
	payload, _ := json.Marshal(commandError{OK: false, Code: code, Message: err.Error()})
	fmt.Fprintln(os.Stderr, string(payload))
	os.Exit(1)
}

func readRequired(path, label string) []byte {
	if strings.TrimSpace(path) == "" {
		fail("invalid_arguments", fmt.Errorf("--%s is required", label))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fail("read_failed", fmt.Errorf("read %s: %w", label, err))
	}
	return data
}

func writeJSON(path string, value any) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fail("json_failed", err)
	}
	data = append(data, '\n')
	if path == "" || path == "-" {
		_, err = os.Stdout.Write(data)
	} else {
		if _, statErr := os.Stat(path); statErr == nil {
			fail("output_exists", fmt.Errorf("refusing to overwrite existing file %q", filepath.Base(path)))
		} else if !os.IsNotExist(statErr) {
			fail("write_failed", statErr)
		}
		var file *os.File
		file, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			_, err = file.Write(data)
			if closeErr := file.Close(); err == nil {
				err = closeErr
			}
		}
	}
	if err != nil {
		fail("write_failed", err)
	}
}

func inspectCommand(args []string) {
	flags := flag.NewFlagSet("inspect", flag.ContinueOnError)
	input := flags.String("input", "", "input DOCX")
	output := flags.String("output", "-", "inspection JSON path or -")
	if err := flags.Parse(args); err != nil {
		fail("invalid_arguments", err)
	}
	inspection, err := docx.InspectBytes(readRequired(*input, "input"))
	if err != nil {
		fail("inspect_failed", err)
	}
	writeJSON(*output, inspection)
}

func addCommentsCommand(args []string) {
	flags := flag.NewFlagSet("add-comments", flag.ContinueOnError)
	input := flags.String("input", "", "input DOCX")
	planPath := flags.String("plan", "", "comment plan JSON")
	output := flags.String("output", "", "new DOCX path")
	auditPath := flags.String("audit", "", "audit JSON path")
	if err := flags.Parse(args); err != nil {
		fail("invalid_arguments", err)
	}
	if *output == "" || *auditPath == "" {
		fail("invalid_arguments", fmt.Errorf("--output and --audit are required"))
	}
	inputAbs, err := filepath.Abs(*input)
	if err != nil {
		fail("invalid_arguments", err)
	}
	outputAbs, err := filepath.Abs(*output)
	if err != nil {
		fail("invalid_arguments", err)
	}
	if inputAbs == outputAbs {
		fail("in_place_forbidden", fmt.Errorf("refusing to overwrite the input DOCX"))
	}
	var plan docx.CommentPlan
	if err := json.Unmarshal(readRequired(*planPath, "plan"), &plan); err != nil {
		fail("invalid_plan", err)
	}
	result, audit, err := docx.AddComments(readRequired(*input, "input"), plan, *output)
	if err != nil {
		fail("add_comments_failed", err)
	}
	if err := docx.WriteOutputAndAudit(*output, *auditPath, result, audit); err != nil {
		fail("write_failed", err)
	}
	writeJSON("-", map[string]any{"ok": true, "output_name": filepath.Base(*output), "audit_name": filepath.Base(*auditPath), "output_sha256": audit.OutputSHA256})
}

func addRedlinesCommand(args []string) {
	flags := flag.NewFlagSet("add-redlines", flag.ContinueOnError)
	input := flags.String("input", "", "input DOCX")
	planPath := flags.String("plan", "", "redline plan JSON")
	output := flags.String("output", "", "new redline DOCX path")
	auditPath := flags.String("audit", "", "audit JSON path")
	if err := flags.Parse(args); err != nil {
		fail("invalid_arguments", err)
	}
	if *output == "" || *auditPath == "" {
		fail("invalid_arguments", fmt.Errorf("--output and --audit are required"))
	}
	inputAbs, err := filepath.Abs(*input)
	if err != nil {
		fail("invalid_arguments", err)
	}
	outputAbs, err := filepath.Abs(*output)
	if err != nil {
		fail("invalid_arguments", err)
	}
	if inputAbs == outputAbs {
		fail("in_place_forbidden", fmt.Errorf("refusing to overwrite the input DOCX"))
	}
	var plan docx.RedlinePlan
	if err := json.Unmarshal(readRequired(*planPath, "plan"), &plan); err != nil {
		fail("invalid_plan", err)
	}
	result, audit, err := docx.AddRedlines(readRequired(*input, "input"), plan, *output)
	if err != nil {
		fail("add_redlines_failed", err)
	}
	if err := docx.WriteOutputAndAudit(*output, *auditPath, result, audit); err != nil {
		fail("write_failed", err)
	}
	writeJSON("-", map[string]any{"ok": true, "output_name": filepath.Base(*output), "audit_name": filepath.Base(*auditPath), "output_sha256": audit.OutputSHA256})
}

func applyRevisionsCommand(args []string) {
	flags := flag.NewFlagSet("apply-revisions", flag.ContinueOnError)
	input := flags.String("input", "", "input DOCX with tracked revisions")
	mode := flags.String("mode", "", "accept or reject")
	output := flags.String("output", "", "new clean DOCX path")
	auditPath := flags.String("audit", "", "audit JSON path")
	if err := flags.Parse(args); err != nil {
		fail("invalid_arguments", err)
	}
	if *output == "" || *auditPath == "" {
		fail("invalid_arguments", fmt.Errorf("--output and --audit are required"))
	}
	inputAbs, err := filepath.Abs(*input)
	if err != nil {
		fail("invalid_arguments", err)
	}
	outputAbs, err := filepath.Abs(*output)
	if err != nil {
		fail("invalid_arguments", err)
	}
	if inputAbs == outputAbs {
		fail("in_place_forbidden", fmt.Errorf("refusing to overwrite the input DOCX"))
	}
	result, audit, err := docx.ApplyRevisions(readRequired(*input, "input"), strings.ToLower(strings.TrimSpace(*mode)), *output)
	if err != nil {
		fail("apply_revisions_failed", err)
	}
	if err := docx.WriteOutputAndAudit(*output, *auditPath, result, audit); err != nil {
		fail("write_failed", err)
	}
	writeJSON("-", map[string]any{"ok": true, "mode": *mode, "output_name": filepath.Base(*output), "audit_name": filepath.Base(*auditPath), "output_sha256": audit.OutputSHA256})
}

func validateCommand(args []string) {
	flags := flag.NewFlagSet("validate", flag.ContinueOnError)
	input := flags.String("input", "", "input DOCX")
	output := flags.String("output", "-", "validation JSON path or -")
	if err := flags.Parse(args); err != nil {
		fail("invalid_arguments", err)
	}
	validation := docx.ValidateBytes(readRequired(*input, "input"))
	writeJSON(*output, validation)
	if !validation.Valid {
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: word-document <inspect|add-comments|add-redlines|apply-revisions|validate> [flags]")
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "inspect":
		inspectCommand(os.Args[2:])
	case "add-comments":
		addCommentsCommand(os.Args[2:])
	case "add-redlines":
		addRedlinesCommand(os.Args[2:])
	case "apply-revisions":
		applyRevisionsCommand(os.Args[2:])
	case "validate":
		validateCommand(os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		usage()
		fail("unknown_command", fmt.Errorf("unknown command %q", os.Args[1]))
	}
}
