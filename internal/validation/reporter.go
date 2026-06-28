package validation

import (
	"encoding/json"
	"os"
)

type JSONReporter struct {
	Path string
}

func (r *JSONReporter) Generate(report *ValidationReport) error {
	w := os.Stdout
	if r.Path != "" {
		f, err := os.Create(r.Path)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}
