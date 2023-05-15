// Licensed to YugabyteDB, Inc. under one or more contributor license
// agreements. See the NOTICE file distributed with this work for
// additional information regarding copyright ownership. Yugabyte
// licenses this file to you under the Mozilla License, Version 2.0
// (the "License"); you may not use this file except in compliance
// with the License.  You may obtain a copy of the License at
// http://mozilla.org/MPL/2.0/.
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package utils

import (
	"encoding/json"
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

// AzureCredentials required for cloud provider
type AzureCredentials struct {
	TenantID       string `json:"tenant_id"`
	SubscriptionID string `json:"subscription_id"`
	ClientSecret   string `json:"client_secret"`
	ClientID       string `json:"client_id"`
	ResourceGroup  string `json:"resource_group"`
}

// gcpGetCredentials retrieves the GCE credentials from env variable and returns gcpCredentials
// struct
func gcpGetCredentials() (GCPCredentials, error) {
	gcsCredsByteArray, err := gcpCredentialsFromEnv()
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

// gcpCredentialsFromEnv retrieves credentials from "GOOGLE_APPLICATION_CREDENTIALS"
func gcpCredentialsFromEnv() ([]byte, error) {
	return gcpCredentialsFromFilePath(os.Getenv(GCPCredentialsEnv))
}

// gcpCredentialsFromFilePath retrieves credentials from any given file path
func gcpCredentialsFromFilePath(filePath string) ([]byte, error) {
	if filePath == "" {
		return nil, fmt.Errorf("%s env variable is empty", GCPCredentialsEnv)
	}
	gcsCredsByteArray, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("Failed reading data from %s: %s", GCPCredentialsEnv, err)
	}
	return gcsCredsByteArray, nil
}

// gcpGetJSONTag returns the JSON field name of the struct field
func gcpGetJSONTag(val reflect.StructField) string {
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

// GcpGetCredentialsAsString returns the GCE JSON file contents as a string
func GcpGetCredentialsAsString() (string, error) {
	gcsCredsJSON, err := gcpGetCredentials()
	if err != nil {
		return "", err
	}
	v := reflect.ValueOf(gcsCredsJSON)
	var gcsCredString string
	gcsCredString = "{ "
	for i := 0; i < v.NumField(); i++ {
		var s string
		field := gcpGetJSONTag(v.Type().Field(i))

		if field == "private_key" {
			valString := strings.Replace(v.Field(i).Interface().(string), "\n", "\\n", -1)
			s = "\"" + field + "\"" + ": " + "\"" + valString + "\""

		} else {
			s = "\"" + field + "\"" + ": " + "\"" + v.Field(i).Interface().(string) + "\""
		}
		if gcsCredString[len(gcsCredString)-2] != '{' {
			gcsCredString = gcsCredString + " , " + s
		} else {
			gcsCredString = gcsCredString + s
		}
	}
	gcsCredString = gcsCredString + "}"
	return gcsCredString, nil
}

// GcpGetCredentialsAsMap returns the GCE JSON file contents as a map
func GcpGetCredentialsAsMap() (map[string]interface{}, error) {
	gcsCredsMap := make(map[string]interface{})
	gcsCredsJSON, err := gcpGetCredentials()
	if err != nil {
		return nil, err
	}
	v := reflect.ValueOf(gcsCredsJSON)
	for i := 0; i < v.NumField(); i++ {
		tag := gcpGetJSONTag(v.Type().Field(i))
		gcsCredsMap[tag] = v.Field(i).Interface().(string)
	}
	return gcsCredsMap, nil
}

// AwsCredentialsFromEnv retrives values of "AWS_ACCESS_KEY_ID" and "AWS_SECRET_ACCESS_KEY" from
// env variables
func AwsCredentialsFromEnv() (awsCreds.Value, error) {

	awsCredentials, err := awsCreds.NewEnvCredentials().Get()
	if err != nil {
		return awsCreds.Value{}, fmt.Errorf("Error getting AWS env credentials %s", err)
	}
	return awsCredentials, nil
}

// AzureStorageCredentialsFromEnv retrives value of "AZURE_STORAGE_SAS_TOKEN" from env variables
func AzureStorageCredentialsFromEnv() (string, error) {
	azureSasToken, isPresent := os.LookupEnv(AzureStorageSasTokenEnv)
	if !isPresent {
		return "", fmt.Errorf("%s env variable not found", AzureStorageSasTokenEnv)
	}
	return azureSasToken, nil
}

// AzureCredentialsFromEnv retrives azure credentials from env variables
func AzureCredentialsFromEnv() (AzureCredentials, error) {

	// get client id, client secret, tenat id, resource group and subscription id
	var azureCreds AzureCredentials
	var isPresentClientID, isPresentClientSecret, isPresentSubscriptionID bool
	var isPresentTenantID, isPresentRG bool
	errorString := "Empty env variable: "
	azureCreds.ClientID, isPresentClientID = os.LookupEnv(AzureClientIDEnv)
	if !isPresentClientID {
		errorString = fmt.Sprintf("%s%s ", errorString, AzureClientIDEnv)
	}
	azureCreds.ClientSecret, isPresentClientSecret = os.LookupEnv(AzureClientSecretEnv)
	if !isPresentClientSecret {
		errorString = fmt.Sprintf("%s%s ", errorString, AzureClientSecretEnv)
	}
	azureCreds.SubscriptionID, isPresentSubscriptionID = os.LookupEnv(AzureSubscriptionIDEnv)
	if !isPresentSubscriptionID {
		errorString = fmt.Sprintf("%s%s ", errorString, AzureSubscriptionIDEnv)
	}
	azureCreds.TenantID, isPresentTenantID = os.LookupEnv(AzureTenantIDEnv)
	if !isPresentTenantID {
		errorString = fmt.Sprintf("%s%s ", errorString, AzureTenantIDEnv)
	}
	azureCreds.ResourceGroup, isPresentRG = os.LookupEnv(AzureRGEnv)
	if !isPresentRG {
		errorString = fmt.Sprintf("%s%s ", errorString, AzureRGEnv)
	}
	if !(isPresentClientID && isPresentClientSecret && isPresentRG &&
		isPresentSubscriptionID && isPresentTenantID) {
		return AzureCredentials{}, fmt.Errorf(errorString)
	}
	return azureCreds, nil
}
