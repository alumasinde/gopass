-- Serializes organization ID allocation across platform instances. The allocator
-- also considers the shared tenant schema's maximum ID to prevent collisions.
CREATE TABLE IF NOT EXISTS platform_id_sequences (
 sequence_name VARCHAR(80) NOT NULL PRIMARY KEY,
 next_value BIGINT UNSIGNED NOT NULL,
 updated_at DATETIME NOT NULL
) ENGINE=InnoDB;
INSERT INTO platform_id_sequences(sequence_name,next_value,updated_at)
VALUES('organization',1,UTC_TIMESTAMP())
ON DUPLICATE KEY UPDATE sequence_name=VALUES(sequence_name);
