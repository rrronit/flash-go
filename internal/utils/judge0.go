package utils

// Judge0LanguageIDToName maps Judge0 language IDs to internal language names.
func Judge0LanguageIDToName(id int) (string, bool) {
	switch id {
	case 54, 105:
		return "cpp", true
	case 62, 91:
		return "java", true
	case 71, 100:
		return "python", true
	case 63, 102:
		return "javascript", true
	case 51:
		return "csharp", true
	case 60, 107:
		return "go", true
	default:
		return "", false
	}
}
