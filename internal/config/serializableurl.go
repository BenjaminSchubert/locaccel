package config

import (
	"errors"
	"net/url"

	"gopkg.in/yaml.v3"
)

var ErrMustBeScalar = errors.New("URL must be a scalar")

type SerializableURL struct {
	URL *url.URL
}

func (s *SerializableURL) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode {
		return ErrMustBeScalar
	}

	parsed, err := url.Parse(node.Value)
	if err != nil {
		return err
	}

	s.URL = parsed
	return nil
}
