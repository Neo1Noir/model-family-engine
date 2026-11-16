package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	engine "github.com/Neo1Noir/model-family-engine"
)

func read(file string, target any) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func write(file string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if file == "" {
		_, err = os.Stdout.Write(data)
		return err
	}
	if err = os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: model-family approve|compile|match [options]")
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	input := flags.String("input", "", "input JSON file")
	out := flags.String("out", "", "new output file; omit to print JSON")
	reviewer := flags.String("reviewer", "", "person recording the approval")
	basis := flags.String("basis", "", "explicit evidence/review decision")
	version := flags.String("version", "", "mapping version for compilation")
	model := flags.String("model", "", "hardware model to resolve")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if *input == "" {
		return fmt.Errorf("--input is required")
	}
	switch args[0] {
	case "approve":
		var rules engine.Rules
		if err := read(*input, &rules); err != nil {
			return err
		}
		result, err := engine.Approve(&rules, *reviewer, *basis)
		if err != nil {
			return err
		}
		return write(*out, result)
	case "compile":
		var rules engine.Rules
		if err := read(*input, &rules); err != nil {
			return err
		}
		result, err := engine.Compile(&rules, *version)
		if err != nil {
			return err
		}
		return write(*out, result)
	case "match":
		var index engine.Index
		if err := read(*input, &index); err != nil {
			return err
		}
		if index.Version == "" || index.Entries == nil {
			return fmt.Errorf("invalid compiled index")
		}
		return write(*out, index.Resolve(*model))
	default:
		return fmt.Errorf("unknown operation: %s", args[0])
	}
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
