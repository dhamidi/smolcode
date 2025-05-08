Run the tests using this command: `go test -tags fts5 ./memory/`

There will be errors.

Here are the tests cases that are relevant:

- search for: `build command` should return documents containing both `build` and `command`
- search for: `build` should return documents containing `build`
- search for: `path/file.txt` should return documents containing the exact file path

Rewrite test cases as necessary to match this behavior.
