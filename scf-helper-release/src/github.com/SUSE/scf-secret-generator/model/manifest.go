package model

import (
	"errors"
	"io"
	"io/ioutil"

	yaml "gopkg.in/yaml.v2"
)

// GeneratorType describes the type of generator used for the configuration value
type GeneratorType string

// These are the available generator types for configuration values
const (
	GeneratorTypePassword      = GeneratorType("Password")      // Password
	GeneratorTypeSSH           = GeneratorType("SSH")           // SSH key
	GeneratorTypeCACertificate = GeneratorType("CACertificate") // CA Certificate
	GeneratorTypeCertificate   = GeneratorType("Certificate")   // Certificate
)

// ValueType describes the type of generator used for the configuration value
type ValueType string

// These are the available generator types for configuration values
const (
	ValueTypeCertificate = ValueType("certificate")
	ValueTypeFingerprint = ValueType("fingerprint")
	ValueTypePrivateKey  = ValueType("private_key")
)

// ConfigurationVariableGenerator describes how to automatically generate values
// for a configuration variable
// WARNING: Avoid re-ordering the fields; that would trigger secrets re-generation because
// it changes the serialized generator input strings. For the same reason additional fields
// should be added with the omitempty attribute.
type ConfigurationVariableGenerator struct {
	ID           string        `json:"id,omitempty" yaml:"id,omitempty"`
	Type         GeneratorType `json:"type" yaml:"type"`
	ValueType    ValueType     `json:"value_type,omitempty" yaml:"value_type,omitempty"`
	SubjectNames []string      `json:"subject_names,omitempty" yaml:"subject_names,omitempty"`
	RoleName     string        `json:"role_name,omitempty" yaml:"role_name,omitempty"`
}

// ConfigurationVariable is a configuration to be exposed to the IaaS
//
// Notes on the fields Type and Internal.
// 1. Type's legal values are `user` and `environment`.
//    `user` is default.
//
//    A `user` CV is rendered into k8s yml config files, etc. to make it available to roles who need it.
//    - An internal CV is rendered to all roles.
//    - A public CV is rendered only to the roles whose templates refer to the CV.
//
//    An `environment` CV comes from a script, not the user. Being
//    internal this way it is not rendered to any configuration files.
//
// 2. Internal's legal values are all YAML boolean values.
//    A public CV is used in templates
//    An internal CV is not, consumed in a script instead.
type ConfigurationVariable struct {
	Name          string                          `yaml:"name"`
	PreviousNames []string                        `yaml:"previous_names"`
	Default       interface{}                     `yaml:"default"`
	Description   string                          `yaml:"description"`
	Example       string                          `yaml:"example"`
	Generator     *ConfigurationVariableGenerator `yaml:"generator"`
	Type          CVType                          `yaml:"type"`
	Internal      bool                            `yaml:"internal,omitempty"`
	Secret        bool                            `yaml:"secret,omitempty"`
	Required      bool                            `yaml:"required,omitempty"`
	Immutable     bool                            `yaml:"immutable,omitempty"`
}

// CVType is the type of the configuration variable; see the constants below
type CVType string

const (
	// CVTypeUser is for user-specified variables (default)
	CVTypeUser = CVType("user")
	// CVTypeEnv is for script-specified variables
	CVTypeEnv = CVType("environment")
)

// Configuration contains information about how to configure the
// resulting images
type Configuration struct {
	Variables []*ConfigurationVariable `yaml:"variables"`
}

// Manifest is the top level of the role manifest file
type Manifest struct {
	Configuration *Configuration `yaml:"configuration"`
}

// GetManifest loads a manifest from file or string
func GetManifest(r io.Reader) (Manifest, error) {
	data, err := ioutil.ReadAll(r)

	var manifest Manifest
	if err == nil {
		err = yaml.Unmarshal(data, &manifest)
		if err == nil && manifest.Configuration == nil {
			err = errors.New("'configuration section' not found in manifest")
		}
	}

	return manifest, err
}
