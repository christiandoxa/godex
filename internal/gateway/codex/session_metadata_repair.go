package codex

func repairSessionMetadataPrefix(path, _ string) (bool, error) {
	selector, ok := sessionIDFromPath(path)
	if !ok {
		return false, nil
	}
	return repairSessionMetadataPrefixWithSelector(path, selector)
}

func repairSessionMetadataPrefixWithSelector(path, selector string) (bool, error) {
	return repairSessionMetadataPrefixWithHook(path, selector, nil)
}

func repairSessionMetadataPrefixAfterPrepare(path, selector string, beforeVerify func()) (bool, error) {
	return repairSessionMetadataPrefixWithHook(path, selector, beforeVerify)
}

func repairSessionMetadataPrefixWithHook(path, selector string, beforeVerify func()) (bool, error) {
	transaction, err := beginSessionRepairTransaction(path)
	if err != nil {
		return false, err
	}
	defer transaction.close()

	repaired, changed, err := planSessionMetadataRepairWithSelector(path, selector, transaction.contents)
	if err != nil {
		return false, err
	}
	if !changed {
		return false, nil
	}
	if err := transaction.commit([]byte(repaired), beforeVerify); err != nil {
		return false, err
	}
	return true, nil
}
