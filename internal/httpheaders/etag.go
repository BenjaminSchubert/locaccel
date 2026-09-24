package httpheaders

func EtagsMatch(etag1, etag2 string) bool {
	if etag1 == etag2 {
		return true
	}
	if len(etag1) > 2 && etag1[:2] == "W/" {
		return etag1[2:] == etag2
	}
	if len(etag2) > 2 && etag2[:2] == "W/" {
		return etag1 == etag2[2:]
	}
	return false
}
