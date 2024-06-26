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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	client "github.com/yugabyte/platform-go-client"
)

// YbaStructuredError is a structure mimicking YBPError, with error being an interface{}
// to accomodate errors thrown as YBPStructuredError
type YbaStructuredError struct {
	// User-visible unstructured error message
	Error *interface{} `json:"error,omitempty"`
	// Method for HTTP call that resulted in this error
	HTTPMethod *string `json:"httpMethod,omitempty"`
	// URI for HTTP request that resulted in this error
	RequestURI *string `json:"requestUri,omitempty"`
	// Mostly set to false to indicate failure
	Success *bool `json:"success,omitempty"`
}

// StringSlice accepts array of interface and returns a pointer to slice of string
func StringSlice(in []interface{}) *[]string {
	var out []string
	for _, v := range in {
		out = append(out, v.(string))
	}
	return &out
}

// StringMap accepts a string -> interface map and returns pointer to string -> string map
func StringMap(in map[string]interface{}) *map[string]string {
	out := make(map[string]string)
	for k, v := range in {
		out[k] = v.(string)
	}
	return &out
}

// MapFromSingletonList returns a map of string -> interface from a slice of interface
func MapFromSingletonList(in []interface{}) map[string]interface{} {
	if len(in) == 0 {
		return make(map[string]interface{})
	}
	return in[0].(map[string]interface{})
}

// MapListFromInterfaceList returns a map of string -> interface from a slice of interface
func MapListFromInterfaceList(in []interface{}) []map[string]interface{} {
	res := make([]map[string]interface{}, 0)
	if len(in) == 0 {
		return res
	}
	for _, i := range in {
		res = append(res, i.(map[string]interface{}))
	}
	return res
}

// GetBoolPointer returns a pointer to bool value
func GetBoolPointer(in bool) *bool {
	return &in
}

// GetStringPointer returns a pointer to string value
func GetStringPointer(in string) *string {
	if in == "" {
		return nil
	}
	return &in
}

// GetInt32Pointer returns a pointer to int32 value
func GetInt32Pointer(in int32) *int32 {
	if in == 0 {
		return nil
	}
	return &in
}

// GetInt64Pointer returns a pointer to int64 value
func GetInt64Pointer(in int64) *int64 {
	if in == 0 {
		return nil
	}
	return &in
}

// GetFloat64Pointer returns a pointer to float64 type
func GetFloat64Pointer(in float64) *float64 {
	if in == 0 {
		return nil
	}
	return &in
}

// CreateSingletonList returns a list of single entry from an interface
func CreateSingletonList(in interface{}) []interface{} {
	return []interface{}{in}
}

var (
	// PendingTaskStates lists incomplete task states
	PendingTaskStates = []string{"Created", "Initializing", "Running"}
	// SuccessTaskStates lists successful task states
	SuccessTaskStates = []string{"Success"}
)

// WaitForTask waits for State change for a YBA task
func WaitForTask(ctx context.Context, tUUID string, cUUID string, c *client.APIClient,
	timeout time.Duration) error {
	wait := &resource.StateChangeConf{
		Delay:   1 * time.Second,
		Pending: PendingTaskStates,
		Target:  SuccessTaskStates,
		Timeout: timeout,

		Refresh: func() (result interface{}, state string, err error) {
			r, response, err := c.CustomerTasksApi.TaskStatus(ctx, cUUID, tUUID).Execute()
			if err != nil {
				errMessage := ErrorFromHTTPResponse(response, err, "Task", "WaitForTask",
					"Get Task Status")
				return nil, "", errMessage
			}
			tflog.Info(ctx, fmt.Sprintf("Task \"%s\" completion percentage: %.0f%%", r["title"].(string),
				r["percent"].(float64)))

			subtasksDetailsList := r["details"].(map[string]interface{})["taskDetails"].([]interface{})
			var subtasksStatus string
			for _, task := range subtasksDetailsList {
				taskMap := task.(map[string]interface{})
				subtasksStatus = fmt.Sprintf("%sTitle: \"%s\", Status: \"%s\"; ",
					subtasksStatus, taskMap["title"].(string), taskMap["state"].(string))
			}
			if subtasksStatus != "" {
				tflog.Info(ctx, fmt.Sprintf("Substasks: %s", subtasksStatus))
			}
			s := r["status"].(string)
			return s, s, nil
		},
	}

	if funcResponse, err := wait.WaitForStateContext(ctx); err != nil {
		allowed, _, errV := failureSubTaskListYBAVersionCheck(ctx, c)
		if errV != nil {
			return errV
		}
		var subtasksFailure string
		if allowed {
			r, response, errR := c.CustomerTasksApi.ListFailedSubtasks(ctx, cUUID, tUUID).Execute()
			if errR != nil {
				errMessage := ErrorFromHTTPResponse(response, errR, "Task", "ListFailedSubtasks",
					"Get Failed Tasks")
				return errMessage
			}

			for _, f := range r.GetFailedSubTasks() {
				subtasksFailure = fmt.Sprintf("%sSubTaskType: \"%s\", Error: \"%s\"; ",
					subtasksFailure, f.GetSubTaskType(), f.GetErrorString())
			}
		} else {
			subtasksFailure = fmt.Sprintln("Please refer to the YugabyteDB Anywhere Tasks",
				"for description")
		}
		if subtasksFailure != "" {
			return fmt.Errorf("State: %s, %s", funcResponse.(string), subtasksFailure)
		}
		return fmt.Errorf("State: %s", funcResponse.(string))
	}

	return nil
}

// YBAMinimumVersion corresponds to the oldest version which allows an operation
// With the new name change, need separate Stable and Preview releases for comparison
type YBAMinimumVersion struct {
	Stable  string
	Preview string
}

// CheckValidYBAVersion allows operation if version is higher than listed versions
func CheckValidYBAVersion(ctx context.Context, c *client.APIClient, versions YBAMinimumVersion) (
	bool, string, error) {

	r, response, err := c.SessionManagementApi.AppVersion(ctx).Execute()
	if err != nil {
		errMessage := ErrorFromHTTPResponse(response, err, "Validation",
			"YBA Version", "Get App Version")
		return false, "", errMessage
	}
	currentVersion := r["version"]
	var v string
	isStable, err := IsVersionStable(currentVersion)
	if err != nil {
		return false, currentVersion, err
	}
	if isStable {
		v = versions.Stable
	} else {
		v = versions.Preview
	}
	check, err := CompareYbVersions(currentVersion, v)
	if err != nil {
		return false, "", err
	}
	if check == 0 || check == 1 {
		return true, currentVersion, err
	}

	return false, currentVersion, err
}

// IsPreviewVersionAllowed checks if a current version (>= Min version)
// is equal to the restricted version for the operation.
// Used in cases where certain preview build errors are not
// resolved and need to be blocked on YugabyteDB Anywhere Terraform
// provider
func IsPreviewVersionAllowed(currentVersion, restrictedVersion string) (bool, error) {
	isStable, err := IsVersionStable(currentVersion)
	if err != nil {
		return true, err
	}
	// Check only with preview builds, stable current versions return true
	if isStable {
		return true, nil
	}
	compare, errCompare := CompareYbVersions(restrictedVersion, currentVersion)
	if errCompare != nil {
		return false, errCompare
	}
	if compare == 0 {
		return false, nil
	}
	return true, nil
}

// IsVersionStable checks if a given version string is on stable track or not.
// Eg: 2024.1.0.0-b1/2.20.0.0-b1 for
// stable and 2.23.0.0-b1 for preview.
func IsVersionStable(version string) (bool, error) {
	v := strings.Split(version, ".")
	v1, err := strconv.Atoi(v[1])
	if err != nil {
		err = fmt.Errorf("Unable to parse YB version strings")
		return false, err
	}
	return (v1%2 == 0 || len(v[0]) == 4), nil
}

// CompareYbVersions returns -1 if version1 < version2, 0 if version1 = version2,
// 1 if version1 > version2
func CompareYbVersions(v1 string, v2 string) (int, error) {
	ybaVersionRegex := "^(\\d+.\\d+.\\d+.\\d+)(-(b(\\d+)|(\\w+)))?$"
	// After the second dash, a user can add anything, and it will be ignored.
	v1Parts := strings.Split(v1, "-")
	if len(v1Parts) > 2 {
		v1 = fmt.Sprintf("%v%v", v1Parts[0]+"-", v1Parts[1])
	}
	v2Parts := strings.Split(v2, "-")
	if len(v2Parts) > 2 {
		v2 = fmt.Sprintf("%v%v", v2Parts[0]+"-", v2Parts[1])
	}
	versionPattern, err := regexp.Compile(ybaVersionRegex)
	if err != nil {
		return 0, err
	}
	v1Matcher := versionPattern.Match([]byte(v1))
	v2Matcher := versionPattern.Match([]byte(v2))
	if v1Matcher && v2Matcher {
		v1Groups := versionPattern.FindAllStringSubmatch(v1, -1)
		v2Groups := versionPattern.FindAllStringSubmatch(v2, -1)
		v1Numbers := strings.Split(v1Groups[0][1], ".")
		v2Numbers := strings.Split(v2Groups[0][1], ".")
		for i := 0; i < 4; i++ {
			var err error
			a, err := strconv.Atoi(v1Numbers[i])
			if err != nil {
				return 0, err
			}
			b, err := strconv.Atoi(v2Numbers[i])
			if err != nil {
				return 0, err
			}
			if a > b {
				return 1, nil
			} else if a < b {
				return -1, nil
			}
		}
		v1BuildNumber := v1Groups[0][4]
		v2BuildNumber := v2Groups[0][4]
		// If one of the build number is null (i.e local build) then consider
		// versions as equal as we cannot compare between local builds
		// e.g: 2.5.2.0-b15 and 2.5.2.0-custom are considered equal
		// 2.5.2.0-custom1 and 2.5.2.0-custom2 are considered equal too
		if v1BuildNumber != "" && v2BuildNumber != "" {
			var err error
			a, err := strconv.Atoi(v1BuildNumber)
			if err != nil {
				return 0, err
			}
			b, err := strconv.Atoi(v2BuildNumber)
			if err != nil {
				return 0, err
			}
			if a > b {
				return 1, nil
			} else if a < b {
				return -1, nil
			} else {
				return 0, nil
			}
		}
		return 0, nil
	}
	return 0, errors.New("Unable to parse YB version strings")
}

// ConvertUnitToMs converts time from unit to milliseconds
func ConvertUnitToMs(value float64, unit string) int64 {
	var v int64
	if strings.Compare(unit, "YEARS") == 0 {
		v = int64(value * 12 * 30 * 24 * 60 * 60 * 1000)
	} else if strings.Compare(unit, "MONTHS") == 0 {
		v = int64(value * 30 * 24 * 60 * 60 * 1000)
	} else if strings.Compare(unit, "DAYS") == 0 {
		v = int64(value * 24 * 60 * 60 * 1000)
	} else if strings.Compare(unit, "HOURS") == 0 {
		v = int64(value * 60 * 60 * 1000)
	} else if strings.Compare(unit, "MINUTES") == 0 {
		v = int64(value * 60 * 1000)
	} else if strings.Compare(unit, "SECONDS") == 0 {
		v = int64(value * 1000)
	}
	return v
}

// ConvertMsToUnit converts time from milliseconds to unit
func ConvertMsToUnit(value int64, unit string) float64 {
	var v float64
	if strings.Compare(unit, "YEARS") == 0 {
		v = (float64(value) / 12 / 30 / 24 / 60 / 60 / 1000)
	} else if strings.Compare(unit, "MONTHS") == 0 {
		v = (float64(value) / 30 / 24 / 60 / 60 / 1000)
	} else if strings.Compare(unit, "DAYS") == 0 {
		v = (float64(value) / 24 / 60 / 60 / 1000)
	} else if strings.Compare(unit, "HOURS") == 0 {
		v = (float64(value) / 60 / 60 / 1000)
	} else if strings.Compare(unit, "MINUTES") == 0 {
		v = (float64(value) / 60 / 1000)
	} else if strings.Compare(unit, "SECONDS") == 0 {
		v = (float64(value) / 1000)
	}
	return v
}

// GetUnitOfTimeFromDuration takes time.Duration as input and caluclates the unit specified in
// that duration
func GetUnitOfTimeFromDuration(duration time.Duration) string {
	if duration.Hours() >= float64(24*30*365) {
		return "YEARS"
	} else if duration.Hours() >= float64(24*30) {
		return "MONTHS"
	} else if duration.Hours() >= float64(24) {
		return "DAYS"
	} else if duration.Hours() >= float64(1) {
		return "HOURS"
	} else if duration.Minutes() >= float64(1) {
		return "MINUTES"
	} else if duration.Seconds() >= float64(1) {
		return "SECONDS"
	} else if duration.Milliseconds() > int64(0) {
		return "MILLISECONDS"
	} else if duration.Microseconds() > int64(0) {
		return "MICROSECONDS"
	} else if duration.Nanoseconds() > int64(0) {
		return "NANOSECONDS"
	}
	return ""
}

// GetMsFromDurationString retrieves the ms notation of the duration mentioned in the input string
// return value string holds the unit calculated from time.Duration
// Throws error on improper duration format
func GetMsFromDurationString(duration string) (int64, string, bool, error) {
	number, err := time.ParseDuration(duration)
	if err != nil {
		return 0, "", false, err
	}
	unitFromDuration := GetUnitOfTimeFromDuration(number)
	return number.Milliseconds(), unitFromDuration, true, err
}

// ErrorFromHTTPResponse extracts the error message from the HTTP response of the API
func ErrorFromHTTPResponse(resp *http.Response, apiError error, entity, entityName,
	operation string) error {
	errorTag := fmt.Errorf("%s: %s, Operation: %s - %w", entity, entityName, operation, apiError)
	if resp == nil {
		return errorTag
	}
	response := *resp
	errorBlock := YbaStructuredError{}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("%w: %s", errorTag, "Error reading HTTP Response body")
	}
	if err = json.Unmarshal(body, &errorBlock); err != nil {
		return fmt.Errorf("%w: %s %s", errorTag,
			"Failed unmarshalling HTTP Response body", err.Error())
	}
	errorString := ErrorFromResponseBody(errorBlock)
	return fmt.Errorf("%w: %s", errorTag, errorString)
}

// ErrorFromResponseBody is a function to extract error interfaces into string
func ErrorFromResponseBody(errorBlock YbaStructuredError) string {
	var errorString string
	if reflect.TypeOf(*errorBlock.Error) == reflect.TypeOf(errorString) {
		return (*errorBlock.Error).(string)
	}

	errorMap := (*errorBlock.Error).(map[string]interface{})
	for k, v := range errorMap {
		if k != "" {
			errorString = fmt.Sprintf("Field: %s, Error:", k)
		}
		var checkType []interface{}
		if reflect.TypeOf(v) == reflect.TypeOf(checkType) {
			for _, s := range *StringSlice(v.([]interface{})) {
				errorString = fmt.Sprintf("%s %s", errorString, s)
			}
		} else {
			errorString = fmt.Sprintf("%s %s", errorString, v.(string))
		}

	}
	return errorString
}

// FileExist checks if file in the given path exists
func FileExist(filePath string) error {
	_, error := os.Stat(filePath)

	// check if error is "file not exists"
	if os.IsNotExist(error) {
		return fmt.Errorf("%s file does not exist", filePath)
	}
	return nil
}

// GetUniversesForProvider fetches the list of universes corresponding to a particular
// provider. Currently edit operations are blocked if universes exists. For the current
// scenario, only on prem providers are editable, but to accomodate future changes to
// cloud provider resource, defining in the utils class
func GetUniversesForProvider(ctx context.Context, c *client.APIClient, cUUID, pUUID,
	universeName string) ([]client.UniverseResp, bool, error) {
	var r []client.UniverseResp
	var response *http.Response
	universeList := make([]client.UniverseResp, 0)
	var err error
	if universeName != "" {
		r, response, err = c.UniverseManagementApi.ListUniverses(ctx, cUUID).Name(universeName).Execute()
		if err != nil {
			errMessage := ErrorFromHTTPResponse(response, err, ResourceEntity,
				"Universe", "Get List of Universes")
			return nil, false, errMessage
		}
	} else {
		r, response, err = c.UniverseManagementApi.ListUniverses(ctx, cUUID).Execute()
		if err != nil {
			errMessage := ErrorFromHTTPResponse(response, err, ResourceEntity,
				"Universe", "Get List of Universes")
			return nil, false, errMessage
		}
	}
	for _, u := range r {
		primary := u.GetUniverseDetails().Clusters[0]
		userIntent := primary.GetUserIntent()
		if pUUID == userIntent.GetProvider() {
			universeList = append(universeList, u)
		}
	}
	if len(universeList) > 0 {
		return universeList, true, err
	}
	return universeList, false, err
}

func failureSubTaskListYBAVersionCheck(ctx context.Context, c *client.APIClient) (
	bool, string, error) {
	allowedVersions := YBAMinimumVersion{
		Stable:  YBAAllowFailureSubTaskListMinVersion,
		Preview: YBAAllowFailureSubTaskListMinVersion}
	allowed, version, err := CheckValidYBAVersion(ctx, c, allowedVersions)
	if err != nil {
		return false, "", err
	}
	if allowed {
		// if the release is 2.19.0.0, block it like YBA < 2.18.1.0 and send generic message
		restrictedVersions := YBARestrictFailedSubtasksVersions()
		for _, i := range restrictedVersions {
			allowed, err = IsPreviewVersionAllowed(version, i)
			if err != nil {
				return false, version, err
			}
		}
	}
	return allowed, version, err
}

// ObfuscateString masks sensitive strings in the state file
func ObfuscateString(s string, n int) string {
	if len(s) < 6 {
		return "*"
	}
	substring := s[n : len(s)-n]
	replaced := strings.Replace(s, substring, strings.Repeat("*", len(substring)), 1)
	return replaced
}
