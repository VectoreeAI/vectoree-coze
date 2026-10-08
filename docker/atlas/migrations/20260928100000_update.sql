-- Modify "user" table
ALTER TABLE `opencoze`.`user` ADD COLUMN `vectoree_issuer` varchar(255) NOT NULL DEFAULT "" COMMENT "Vectoree Auth issuer", ADD COLUMN `vectoree_subject` varchar(255) NOT NULL DEFAULT "" COMMENT "Vectoree Auth user id", ADD INDEX `idx_vectoree_identity` (`vectoree_issuer`, `vectoree_subject`);
