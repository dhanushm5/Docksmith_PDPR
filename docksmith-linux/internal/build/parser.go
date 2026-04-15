package build

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Instruction struct {
	Op   string
	Args []string
	Raw  string
}

type File struct {
	Instructions []Instruction
}

var allowedOps = map[string]bool{
	"FROM":    true,
	"COPY":    true,
	"RUN":     true,
	"WORKDIR": true,
	"ENV":     true,
	"CMD":     true,
}

func ParseDocksmithfile(contextDir string) (File, error) {
	path := filepath.Join(contextDir, "Docksmithfile")
	f, err := os.Open(path)
	if err != nil {
		return File{}, fmt.Errorf("open Docksmithfile: %w", err)
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	lineNo := 0
	var out File
	for s.Scan() {
		lineNo++
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		op := strings.ToUpper(fields[0])
		if !allowedOps[op] {
			return File{}, fmt.Errorf("line %d: unsupported instruction %q", lineNo, op)
		}

		ins := Instruction{Op: op, Raw: line}
		switch op {
		case "RUN", "CMD":
			if len(fields) < 2 {
				return File{}, fmt.Errorf("line %d: %s requires arguments", lineNo, op)
			}
			ins.Args = []string{strings.TrimSpace(line[len(fields[0]):])}
		default:
			if len(fields) < 2 {
				return File{}, fmt.Errorf("line %d: %s requires arguments", lineNo, op)
			}
			ins.Args = fields[1:]
		}
		out.Instructions = append(out.Instructions, ins)
	}
	if err := s.Err(); err != nil {
		return File{}, fmt.Errorf("read Docksmithfile: %w", err)
	}
	if len(out.Instructions) == 0 {
		return File{}, fmt.Errorf("Docksmithfile has no instructions")
	}
	if out.Instructions[0].Op != "FROM" {
		return File{}, fmt.Errorf("Docksmithfile must start with FROM")
	}
	return out, nil
}
