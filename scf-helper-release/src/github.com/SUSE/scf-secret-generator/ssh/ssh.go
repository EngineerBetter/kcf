package ssh

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"

	"golang.org/x/crypto/ssh"
	"k8s.io/api/core/v1"

	"github.com/SUSE/scf-secret-generator/model"
	"github.com/SUSE/scf-secret-generator/util"
)

// Key describes a key/fingerprint pair
type Key struct {
	PrivateKey  string // Name to associate with private key
	Fingerprint string // Name to associate with fingerprint
}

// RecordKeyInfo records priave key or fingerprint names for later generation
func RecordKeyInfo(keys map[string]Key, configVar *model.ConfigurationVariable) error {
	if len(configVar.Generator.ID) == 0 {
		return fmt.Errorf("Config variable `%s` has no ID value", configVar.Name)
	}
	if configVar.Generator.Type != model.GeneratorTypeSSH {
		return fmt.Errorf("Config variable `%s` does not have a valid SSH generator type", configVar.Name)
	}

	// Get or create the key from the map, there should always be
	// a pair of private keys and fingerprints
	key := keys[configVar.Generator.ID]

	switch configVar.Generator.ValueType {
	case model.ValueTypeFingerprint:
		if len(key.Fingerprint) > 0 {
			return fmt.Errorf("Multiple variables define fingerprints name for SSH id `%s`", configVar.Generator.ID)
		}
		key.Fingerprint = configVar.Name
	case model.ValueTypePrivateKey:
		if len(key.PrivateKey) > 0 {
			return fmt.Errorf("Multiple variables define private key name for SSH id `%s`", configVar.Generator.ID)
		}
		key.PrivateKey = configVar.Name
	default:
		return fmt.Errorf("Config variable `%s` has invalid value type `%s`", configVar.Name, configVar.Generator.ValueType)
	}

	keys[configVar.Generator.ID] = key
	return nil
}

// GenerateAllKeys will create private keys and fingerprints for all recorded SSH variables
func GenerateAllKeys(keys map[string]Key, secrets *v1.Secret) error {
	for id, key := range keys {
		if len(key.PrivateKey) == 0 {
			return fmt.Errorf("No private key name defined for SSH id `%s`", id)
		}
		if len(key.Fingerprint) == 0 {
			return fmt.Errorf("No fingerprint name defined for SSH id `%s`", id)
		}
		err := generateKey(secrets, key)
		if err != nil {
			return err
		}
	}
	return nil
}

// generateKey will create a single private key and fingerprint pair
func generateKey(secrets *v1.Secret, key Key) error {
	secretKey := util.ConvertNameToKey(key.PrivateKey)
	fingerprintKey := util.ConvertNameToKey(key.Fingerprint)

	// Only create keys, don't update them
	if len(secrets.Data[secretKey]) > 0 {
		return nil
	}

	log.Printf("- SSH priK: %s\n", key.PrivateKey)

	// generate private key
	private, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return err
	}

	privateBlock := &pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: nil,
		Bytes:   x509.MarshalPKCS1PrivateKey(private),
	}

	// generate MD5 fingerprint
	public, err := ssh.NewPublicKey(&private.PublicKey)
	if err != nil {
		return err
	}

	// PEM encode private key
	secrets.Data[secretKey] = pem.EncodeToMemory(privateBlock)
	secrets.Data[fingerprintKey] = []byte(ssh.FingerprintLegacyMD5(public))

	return nil
}
