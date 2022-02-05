package utils

func StringSlice(in []interface{}) (out []string) {
	for _, v := range in {
		out = append(out, v.(string))
	}
	return out
}

func StringMap(in map[string]interface{}) map[string]string {
	out := make(map[string]string)
	for k, v := range in {
		out[k] = v.(string)
	}
	return out
}

var PendingTaskStates = []string{"Created", "Initializing", "Running"}
var SuccessTaskStates = []string{"Success"}
