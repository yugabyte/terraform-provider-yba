package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strings"

	awsCreds "github.com/aws/aws-sdk-go/aws/credentials"
)

// GCPCredentials is a struct to hold values retrieved by parsing the GCE credentials json file
type GCPCredentials struct {
	AuthProviderX509CertURL string `json:"auth_provider_x509_cert_url,omitempty"`
	AuthURI                 string `json:"auth_uri,omitempty"`
	ClientEmail             string `json:"client_email"`
	ClientID                string `json:"client_id"`
	ClientX509CertURL       string `json:"client_x509_cert_url"`
	PrivateKey              string `json:"private_key"`
	PrivateKeyID            string `json:"private_key_id"`
	ProjectID               string `json:"project_id"`
	TokenURI                string `json:"token_uri,omitempty"`
	Type                    string `json:"type"`
}

// GetGcpCredentials retrieves the GCE credentials from env variable and returns GCPCredentials
// struct
func GetGcpCredentials() (GCPCredentials, error) {
	gcsCredsByteArray, err := GcpCredentialsFromEnv()
	if err != nil {
		return GCPCredentials{}, err
	}

	gcsCredsJSON := GCPCredentials{}
	err = json.Unmarshal(gcsCredsByteArray, &gcsCredsJSON)
	if err != nil {
		return GCPCredentials{}, fmt.Errorf("Failed unmarshalling GCE credentials: %s", err)
	}
	return gcsCredsJSON, nil

}

// GcpCredentialsFromEnv retrieves credentials from "GOOGLE_APPLICATION_CREDENTIALS"
func GcpCredentialsFromEnv() ([]byte, error) {
	return GcpCredentialsFromFilePath(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"))
}

// GcpCredentialsFromFilePath retrieves credentials from any given file path
func GcpCredentialsFromFilePath(filePath string) ([]byte, error) {
	if filePath == "" {
		return nil, fmt.Errorf("GOOGLE_APPLICATION_CREDENTIALS env variable is empty")
	}
	gcsCredsByteArray, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("Failed reading data from GOOGLE_APPLICATION_CREDENTIALS: %s", err)
	}
	return gcsCredsByteArray, nil
}

// GcsGetJSONTag returns the JSON field name of the struct field
func GcsGetJSONTag(val reflect.StructField) string {
	switch jsonTag := val.Tag.Get("json"); jsonTag {
	case "-":
	case "":
		return val.Name
	default:
		parts := strings.Split(jsonTag, ",")
		name := parts[0]
		if name == "" {
			name = val.Name
		}
		return name
	}
	return ""
}

// AwsCredentialsFromEnv retrives "AWS_ACCESS_KEY_ID" and "AWS_SECRET_ACCESS_KEY" from env
// variables
func AwsCredentialsFromEnv() (awsCreds.Value, error) {

	awsCredentials, err := awsCreds.NewEnvCredentials().Get()
	if err != nil {
		return awsCreds.Value{}, fmt.Errorf("Error getting AWS env credentials %s", err)
	}
	return awsCredentials, nil
}

// AzureCredentialsFromEnv retrives "AZURE_STORAGE_SAS_TOKEN" from env
// variables
func AzureCredentialsFromEnv() (string, error) {
	azureSasToken, isPresent := os.LookupEnv("AZURE_STORAGE_SAS_TOKEN")
	if !isPresent {
		return "", errors.New("AZURE_STORAGE_SAS_TOKEN env variable not found")
	}
	return azureSasToken, nil
}
