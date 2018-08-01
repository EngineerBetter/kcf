package password

import (
	"log"

	"github.com/SUSE/scf-secret-generator/util"
	"github.com/dchest/uniuri"
	"k8s.io/api/core/v1"
)

// GeneratePassword generates a password for `secretName` if it doesn't already exist
func GeneratePassword(secrets *v1.Secret, secretName string) {
	secretKey := util.ConvertNameToKey(secretName)

	// Only create keys, don't update them
	if len(secrets.Data[secretKey]) == 0 {
		log.Printf("- Password: %s\n", secretName)

		password := uniuri.NewLen(64)
		secrets.Data[secretKey] = []byte(password)
	}
}
