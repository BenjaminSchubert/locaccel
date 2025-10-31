package main

import (
	"bufio"
	"fmt"
	"os"
	"slices"
	"strings"
	"text/template"
)

type Result struct {
	TestName   string
	BaseValue  string
	HeadValue  string
	Comparison string
	PValue     string
}

func isNewTable(line string) bool {
	entries := strings.Split(line, ",")
	for _, idx := range []int{0, 2, 4, 5, 6} {
		if entries[idx] != "" {
			return false
		}
	}
	return entries[1] != "" && entries[3] != ""
}

func computeValue(value, ci string) string {
	if value == "" {
		return "n/a"
	}
	if ci == "" {
		return value
	}
	return value + " ± " + ci
}

func parseTable( //nolint:gocritic
	lines []string,
	root string,
) (goos, goarch, cpu, base, head, unit, pkg string, results []Result) {
outer:
	for idx, line := range lines {
		switch {
		case strings.HasPrefix(line, "goos: "):
			if goos != "" {
				panic("Unexpected entry: 'goos'. Already have an os.")
			}
			goos = line[6:]
		case strings.HasPrefix(line, "goarch: "):
			if goarch != "" {
				panic("Unexpected entry: 'goarch'. Already have an arch.")
			}
			goarch = line[8:]
		case strings.HasPrefix(line, "cpu: "):
			if cpu != "" {
				panic("Unexpected entry: 'cpu'. Already have a cpu.")
			}
			cpu = line[5:]
		case strings.HasPrefix(line, "pkg: "):
			if pkg != "" {
				panic("Unexpected entry: 'pkg'. Already have a pkg.")
			}
			pkg = strings.TrimPrefix(line[5:], root)
		case isNewTable(line):
			lines = lines[idx:]
			break outer
		default:
			panic("Unable to handle line: " + line)
		}
	}

	entries := strings.Split(lines[0], ",")
	base = entries[1]
	head = entries[3]

	unit = strings.Split(lines[1], ",")[1]

	for _, row := range lines[2:] {
		cols := strings.Split(row, ",")
		if len(cols) != 7 {
			panic("Unexpected length for row: " + row)
		}

		r := Result{
			cols[0],
			computeValue(cols[1], cols[2]),
			computeValue(cols[3], cols[4]),
			cols[5],
			cols[6],
		}

		if r.TestName == "geomean" {
			results = append([]Result{r}, results...)
		} else {
			results = append(results, r)
		}
	}

	return goos, goarch, cpu, base, head, unit, pkg, results
}

func main() {
	root := strings.ToLower(os.Args[1])
	goos := ""
	goarch := ""
	cpu := ""
	baseFile := ""
	headFile := ""
	pkg := ""
	units := make([]string, 0)
	resultsPerUnit := make(map[string]map[string][]Result)

	scanner := bufio.NewScanner(os.Stdin)
	lines := make([]string, 0)

	for scanner.Scan() {
		if line := scanner.Text(); line != "" {
			lines = append(lines, line)
			continue
		}

		nGoos, nGoarch, nCpu, nBase, nHead, unit, nPkg, results := parseTable(lines, root)
		if nGoos != "" {
			if goos != "" {
				panic(fmt.Sprintf("Found two different goos? %s vs %s", goos, nGoos))
			}
			goos = nGoos
		}
		if nGoarch != "" {
			if goarch != "" {
				panic(fmt.Sprintf("Found two different goarch? %s vs %s", goarch, nGoarch))
			}
			goarch = nGoarch
		}
		if nCpu != "" {
			if cpu != "" {
				panic(fmt.Sprintf("Found two different cpu? %s vs %s", cpu, nCpu))
			}
			cpu = nCpu
		}
		if baseFile == "" {
			baseFile = nBase
		} else if baseFile != nBase {
			panic(fmt.Sprintf("Found two different base files? %s vs %s", baseFile, nBase))
		}
		if headFile == "" {
			headFile = nHead
		} else if headFile != nHead {
			panic(fmt.Sprintf("Found two different head files? %s vs %s", headFile, nHead))
		}
		if nPkg != "" {
			pkg = nPkg
		}

		if !slices.Contains(units, unit) {
			units = append(units, unit)
		}

		if resultsPerUnit[unit] == nil {
			resultsPerUnit[unit] = make(map[string][]Result)
		}
		resultsPerUnit[unit][pkg] = append(resultsPerUnit[unit][pkg], results...)

		// And reset
		lines = lines[:0]
	}
	if err := scanner.Err(); err != nil {
		panic(err)
	}

	tmpl := template.Must(template.New("result").Parse(`### Environment:

- OS: {{ .os }}
- Arch: {{ .cpu }}
- Goarch: {{ .goarch }}

{{ range $unit := .units -}}
### {{ $unit }}

|Package   |Test|{{ $.baseFile }}|{{ $.headFile }}|vs {{ $.baseFile }}|
|----------|----|----------------|----------------|-------------------|
{{ range $pkg, $results := index $.results $unit -}}
{{ range $idx, $r := $results -}}
{{ if eq $idx 0 -}}
|{{ $pkg }}| **{{ $r.TestName }}** | **{{ $r.BaseValue }}**|**{{ $r.HeadValue }}** | **{{ $r.Comparison }}** |
{{ else -}}
||{{ $r.TestName }}|{{ $r.BaseValue }}|{{ $r.HeadValue }}|{{ $r.Comparison }} ({{ $r.PValue }})|
{{ end -}}
{{ end -}}
{{ end }}
{{ end }}
`))

	if err := tmpl.Execute(os.Stdout, map[string]any{
		"os":       goos,
		"cpu":      cpu,
		"goarch":   goarch,
		"baseFile": baseFile,
		"headFile": headFile,
		"units":    units,
		"results":  resultsPerUnit,
	}); err != nil {
		panic(err)
	}
}
