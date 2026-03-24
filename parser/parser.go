package parser

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Instruction struct {
	Type string   // FROM, COPY, RUN, WORKDIR, ENV, CMD
	Args []string // Parsed arguments
	Raw  string   // Raw instruction text
	Line int      // Line number in Docksmithfile
}

type Docksmithfile struct {
	Instructions []*Instruction
	Path         string
}

// Parse reads and parses a Docksmithfile
func Parse(filePath string) (*Docksmithfile, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open Docksmithfile: %w", err)
	}
	defer file.Close()

	df := &Docksmithfile{
		Instructions: []*Instruction{},
		Path:         filePath,
	}

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse instruction
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}

		instrType := strings.ToUpper(parts[0])
		args := parts[1:]

		switch instrType {
		case "FROM":
			if len(args) != 1 {
				return nil, fmt.Errorf("line %d: FROM requires exactly one argument", lineNum)
			}
			df.Instructions = append(df.Instructions, &Instruction{
				Type: instrType,
				Args: args,
				Raw:  line,
				Line: lineNum,
			})

		case "COPY":
			if len(args) < 2 {
				return nil, fmt.Errorf("line %d: COPY requires at least source and destination", lineNum)
			}
			df.Instructions = append(df.Instructions, &Instruction{
				Type: instrType,
				Args: args,
				Raw:  line,
				Line: lineNum,
			})

		case "RUN":
			if len(args) == 0 {
				return nil, fmt.Errorf("line %d: RUN requires a command", lineNum)
			}
			// RUN command can contain multiple words
			df.Instructions = append(df.Instructions, &Instruction{
				Type: instrType,
				Args: []string{strings.Join(args, " ")},
				Raw:  line,
				Line: lineNum,
			})

		case "WORKDIR":
			if len(args) != 1 {
				return nil, fmt.Errorf("line %d: WORKDIR requires exactly one argument", lineNum)
			}
			df.Instructions = append(df.Instructions, &Instruction{
				Type: instrType,
				Args: args,
				Raw:  line,
				Line: lineNum,
			})

		case "ENV":
			if len(args) != 1 || !strings.Contains(args[0], "=") {
				return nil, fmt.Errorf("line %d: ENV requires KEY=VALUE format", lineNum)
			}
			df.Instructions = append(df.Instructions, &Instruction{
				Type: instrType,
				Args: args,
				Raw:  line,
				Line: lineNum,
			})

		case "CMD":
			if len(args) == 0 {
				return nil, fmt.Errorf("line %d: CMD requires arguments", lineNum)
			}
			// CMD requires JSON array format
			if !strings.HasPrefix(line, "CMD [") {
				return nil, fmt.Errorf("line %d: CMD must be in JSON array format: CMD [\"exec\", \"arg\"]", lineNum)
			}
			// Extract JSON array from line
			cmdLine := strings.TrimPrefix(strings.TrimSpace(line), "CMD ")
			var cmdArray []string
			if err := json.Unmarshal([]byte(cmdLine), &cmdArray); err != nil {
				return nil, fmt.Errorf("line %d: invalid CMD JSON array: %v", lineNum, err)
			}
			df.Instructions = append(df.Instructions, &Instruction{
				Type: instrType,
				Args: cmdArray,
				Raw:  line,
				Line: lineNum,
			})

		default:
			return nil, fmt.Errorf("line %d: unknown instruction: %s", lineNum, instrType)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading Docksmithfile: %w", err)
	}

	if len(df.Instructions) == 0 {
		return nil, fmt.Errorf("Docksmithfile is empty or has no valid instructions")
	}

	if df.Instructions[0].Type != "FROM" {
		return nil, fmt.Errorf("first instruction must be FROM")
	}

	return df, nil
}

// FindDocksmithfile searches for Docksmithfile in the given context directory
func FindDocksmithfile(contextDir string) (string, error) {
	filePath := filepath.Join(contextDir, "Docksmithfile")
	if _, err := os.Stat(filePath); err != nil {
		return "", fmt.Errorf("Docksmithfile not found in %s", contextDir)
	}
	return filePath, nil
}
