# Full-text Query Syntax

The following block contains a summary of the FTS query syntax in BNF form. A detailed explanation follows.

```
<phrase>    := string [*]
<phrase>    := <phrase> + <phrase>
<neargroup> := NEAR ( <phrase> <phrase> ... [, N] )
<query>     := [ [-] <colspec> :] [^] <phrase>
<query>     := [ [-] <colspec> :] <neargroup>
<query>     := [ [-] <colspec> :] ( <query> )
<query>     := <query> AND <query>
<query>     := <query> OR <query>
<query>     := <query> NOT <query>
<colspec>   := colname
<colspec>   := { colname1 colname2 ... }
```

# FTS5 Strings

Within an FTS expression a string may be specified in one of two ways:

By enclosing it in double quotes ("). Within a string, any embedded double quote characters may be escaped SQL-style - by adding a second double-quote character.

As an FTS5 bareword that is not "AND", "OR" or "NOT" (case sensitive). An FTS5 bareword is a string of one or more consecutive characters that are all either:

Non-ASCII range characters (i.e. unicode codepoints greater than 127), or
One of the 52 upper and lower case ASCII characters, or
One of the 10 decimal digit ASCII characters, or
The underscore character (unicode codepoint 95).
The substitute character (unicode codepoint 26).
Strings that include any other characters must be quoted. Characters that are not currently allowed in barewords, are not quote characters and do not currently serve any special purpose in FTS5 query expressions may at some point in the future be allowed in barewords or used to implement new query functionality. This means that queries that are currently syntax errors because they include such a character outside of a quoted string may be interpreted differently by some future version of FTS5.
