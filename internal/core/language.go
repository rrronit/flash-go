package core

// Language describes how to compile and run a job.
type Language struct {
	Name       string `json:"name"`
	SourceFile string `json:"source_file"`
	CompileCmd string `json:"compile_cmd"`
	RunCmd     string `json:"run_cmd"`
	IsCompiled bool   `json:"is_compiled"`
}

// LanguageFor returns the language configuration for a given name.
func LanguageFor(name string) (Language, bool) {
	switch name {
	case "python":
		return Language{
			Name:       "python",
			SourceFile: "main.py",
			CompileCmd: "",
			RunCmd:     "/usr/bin/python3 main.py",
			IsCompiled: false,
		}, true
	case "cpp":
		return Language{
			Name:       "cpp",
			SourceFile: "main.cpp",
			CompileCmd: "/usr/bin/g++ -O0 -Wall -Wextra -Werror -Wpedantic -Wfatal-errors main.cpp",
			RunCmd:     "./a.out",
			IsCompiled: true,
		}, true
	case "javascript":
		return Language{
			Name:       "javascript",
			SourceFile: "main.js",
			CompileCmd: "",
			RunCmd:     "/usr/bin/node main.js",
			IsCompiled: false,
		}, true
	case "java":
		return Language{
			Name:       "java",
			SourceFile: "Main.java",
			CompileCmd: "/usr/bin/javac Main.java",
			RunCmd:     "/usr/bin/java Main",
			IsCompiled: false,
		}, true
	
	default:
		return Language{}, false
	}
}

