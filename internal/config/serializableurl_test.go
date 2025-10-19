package config_test

import (
	"bytes"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/benjaminschubert/locaccel/internal/config"
)

func TestCanConvertFromYaml(t *testing.T) {
	t.Parallel()

	s := config.SerializableURL{}
	decoder := yaml.NewDecoder(bytes.NewBufferString("http://locaccel.test"))

	err := decoder.Decode(&s)
	require.NoError(t, err)
	require.Equal(t, config.SerializableURL{&url.URL{Scheme: "http", Host: "locaccel.test"}}, s)
}

func TestReportErrorOnNonStringUrl(t *testing.T) {
	t.Parallel()

	s := config.SerializableURL{}
	decoder := yaml.NewDecoder(bytes.NewBufferString("[]"))
	err := decoder.Decode(&s)
	require.ErrorIs(t, err, config.ErrMustBeScalar)
}

func TestReportErrorIfInvalidURLProvided(t *testing.T) {
	t.Parallel()

	s := config.SerializableURL{}
	decoder := yaml.NewDecoder(bytes.NewBufferString("://hello:1234"))
	err := decoder.Decode(&s)
	require.ErrorContains(t, err, "missing protocol scheme")
}

func TestCanConvertToYaml(t *testing.T) {
	t.Parallel()

	buf := bytes.NewBufferString("")
	s := config.SerializableURL{&url.URL{Scheme: "https", Host: "locaccel.test"}}
	err := yaml.NewEncoder(buf).Encode(s)
	require.NoError(t, err)
	require.Equal(t, "https://locaccel.test\n", buf.String())
}
