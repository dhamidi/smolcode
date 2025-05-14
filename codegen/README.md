# Codegen Package

This package provides tools to generate code using the Inceptionlabs API.

## Usage

First, initialize a new generator with your API key:

```go
import "github.com/dhamidi/smolcode/codegen" // Adjust import path to your module

apiKey := "YOUR_INCEPTIONLABS_API_KEY"
generator := codegen.New(apiKey)
```

To generate code, provide an instruction and optionally, a slice of existing files:

```go
instruction := "Create a Go function that adds two integers."
existingFiles := []codegen.File{
    // {
    //  Path: "helper.go", 
    //  Contents: []byte("package main\n\nfunc helperUtil(){}"),
    // },
}

files, err := generator.GenerateCode(instruction, existingFiles)
if err != nil {
    // Handle error
    log.Fatalf("Error generating code: %v", err)
}

// The 'files' slice now contains the generated code an paths.
// Each File struct has a Path (string) and Contents ([]byte).
```

To write the generated files to disk (this will overwrite existing files at the specified paths):

```go
err = generator.Write(files)
if err != nil {
    // Handle error
    log.Fatalf("Error writing files: %v", err)
}

fmt.Println("Successfully wrote generated files!")
for _, f := range files {
    fmt.Printf("- Wrote %s\n", f.Path)
}
```

## File Struct

The `File` struct represents a file to be generated or an existing file:

```go
type File struct {
    Path     string `json:"path"`
    Contents []byte `json:"contents"`
}
```

## API Key

Ensure your Inceptionlabs API key is correctly passed to the `New` function. It is recommended to load the API key from an environment variable or a secure configuration file rather than hardcoding it.

For more details on the API itself, see `docs/api_integration.md`.
