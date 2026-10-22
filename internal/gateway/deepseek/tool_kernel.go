package deepseek

func deepSeekResponseKernelAdmits(call map[string]any, name, arguments string) bool {
	id, _ := call["id"].(string)
	signature := deepSeekResponseRawSignature(call)
	return len(id) <= deepSeekResponseNameMaxBytes && len(signature) <= deepSeekResponseNameMaxBytes &&
		len(id)+len(name)+len(arguments)+len(signature) <= deepSeekResponseArgumentsMaxBytes
}
