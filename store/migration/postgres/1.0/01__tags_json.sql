-- Convert space-separated tags to JSON arrays.
UPDATE shortcut
SET tag = CASE
    WHEN trim(tag) = ''  THEN '[]'
    WHEN tag LIKE '[%'   THEN tag
    ELSE '["' || replace(trim(tag), ' ', '","') || '"]'
END;