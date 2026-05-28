-- Convert space-separated tags to JSON arrays.
-- Examples: "" → "[]", "go python" → '["go","python"]', "go" → '["go"]'
UPDATE shortcut
SET tag = CASE
    WHEN trim(tag) = ''      THEN '[]'
    WHEN tag LIKE '[%'       THEN tag
    ELSE '["' || replace(trim(tag), ' ', '","') || '"]'
END;

UPDATE collection
SET tag = CASE
    WHEN trim(tag) = ''      THEN '[]'
    WHEN tag LIKE '[%'       THEN tag
    ELSE '["' || replace(trim(tag), ' ', '","') || '"]'
END;