package evals

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

const DefaultCorpusPath = "corpus/v0.3.0-alpha.1.json"

//go:embed corpus/*.json schema/*.json
var files embed.FS

func LoadDefaultCorpus() (Corpus, error) {
	data, err := files.ReadFile(DefaultCorpusPath)
	if err != nil {
		return Corpus{}, fmt.Errorf("evals: read embedded corpus: %w", err)
	}
	return DecodeCorpus(bytes.NewReader(data))
}

func LoadCorpusFile(path string) (Corpus, error) {
	file, err := os.Open(path)
	if err != nil {
		return Corpus{}, fmt.Errorf("evals: open corpus: %w", err)
	}
	defer file.Close()
	return DecodeCorpus(file)
}

func DecodeCorpus(reader io.Reader) (Corpus, error) {
	if reader == nil {
		return Corpus{}, errors.New("evals: corpus reader is nil")
	}
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var corpus Corpus
	if err := decoder.Decode(&corpus); err != nil {
		return Corpus{}, fmt.Errorf("evals: decode corpus: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Corpus{}, errors.New("evals: corpus contains multiple JSON values")
		}
		return Corpus{}, fmt.Errorf("evals: decode trailing corpus data: %w", err)
	}
	return corpus, nil
}

func JSONSchema() ([]byte, error) {
	data, err := files.ReadFile("schema/corpus.schema.json")
	if err != nil {
		return nil, fmt.Errorf("evals: read embedded schema: %w", err)
	}
	return append([]byte(nil), data...), nil
}
