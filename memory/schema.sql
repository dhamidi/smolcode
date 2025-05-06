CREATE TABLE IF NOT EXISTS memories (
    docid INTEGER PRIMARY KEY AUTOINCREMENT, -- Integer primary key for FTS
    id TEXT UNIQUE NOT NULL, -- User-facing string ID
    content TEXT NOT NULL
);

CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
    content,             -- Column to be indexed from 'memories'
    content='memories',    -- External content table is 'memories'
    content_rowid='docid'  -- Link to 'docid' INTEGER PRIMARY KEY of 'memories'
);

-- Triggers to keep FTS table synchronized with the memories table
CREATE TRIGGER IF NOT EXISTS memories_ai AFTER INSERT ON memories BEGIN
    INSERT INTO memories_fts (rowid, content) VALUES (new.docid, new.content);
END;
CREATE TRIGGER IF NOT EXISTS memories_ad AFTER DELETE ON memories BEGIN
    INSERT INTO memories_fts (memories_fts, rowid, content) VALUES ('delete', old.docid, old.content);
END;
CREATE TRIGGER IF NOT EXISTS memories_au AFTER UPDATE ON memories BEGIN
    INSERT INTO memories_fts (memories_fts, rowid, content) VALUES ('delete', old.docid, old.content);
    INSERT INTO memories_fts (rowid, content) VALUES (new.docid, new.content);
END;
