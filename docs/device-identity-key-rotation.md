# Device identity master-key rotation

`device-identity.master-key` encrypts every managed device permanent credential. Replacing it directly makes all existing credentials undecryptable and causes managed login/bootstrap to fail closed.

For the current schema (`key_version = 1`), rotation is an offline maintenance operation:

1. Stop all API instances and back up the database and current key from the secret manager.
2. Run a dedicated rotation job that decrypts every non-archived `device_identities` row with the old key, encrypts it with the new key using a fresh nonce and the existing `allocation_uuid:rustdesk_id` AAD, and changes `key_version` in the same database transaction.
3. Verify every migrated row can be decrypted with the new key before committing.
4. Deploy the new key to every API instance, start one instance, verify an existing-device bootstrap, then restore normal capacity.
5. Retain the old key only in the protected rollback backup for the approved retention period.

Do not log plaintext credentials or place either key in Git, command-line arguments, migration tables, or application logs. There is currently no online dual-key rotation; do not rotate by merely editing `config.yaml`.
