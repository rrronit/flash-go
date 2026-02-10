package core

import "flash-go/internal/models"

// LanguageFor returns the language configuration for a given name.
func LanguageFor(name string) (models.Language, bool) {
	switch name {
	case "python":
		return models.Language{
			Name:       "python",
			SourceFile: "main.py",
			CompileCmd: "",
			RunCmd:     "/usr/bin/python3 main.py",
			IsCompiled: false,
		}, true
	case "cpp":
		return models.Language{
			Name:       "cpp",
			SourceFile: "main.cpp",
			CompileCmd: "/usr/bin/g++ -O0 -Wall -Wextra -Werror -Wpedantic -Wfatal-errors main.cpp",
			RunCmd:     "./a.out",
			IsCompiled: true,
		}, true
	case "javascript":
		return models.Language{
			Name:       "javascript",
			SourceFile: "main.js",
			CompileCmd: "",
			RunCmd:     "/usr/bin/node main.js",
			IsCompiled: false,
		}, true
	case "java":
		return models.Language{
			Name:       "java",
			SourceFile: "Main.java",
			CompileCmd: "/usr/bin/javac Main.java",
			RunCmd:     "/usr/bin/java Main",
			IsCompiled: true,
		}, true
	case "csharp":
		return models.Language{
			Name:       "csharp",
			SourceFile: "main.cs",
			CompileCmd: "/usr/bin/mcs -optimize+ -out:main.exe main.cs",
			RunCmd:     "/usr/bin/mono main.exe",
			IsCompiled: true,
		}, true
	case "go":
		return models.Language{
			Name:       "go",
			SourceFile: "main.go",
			CompileCmd: "GO111MODULE=off /usr/bin/go build -o main main.go",
			RunCmd:     "./main",
			IsCompiled: true,
		}, true
	default:
		return models.Language{}, false
	}
}
