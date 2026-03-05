package types

const (
	// EventTypeSubmitMatchCertificate is emitted for successful certificate submissions.
	EventTypeSubmitMatchCertificate = "submit_match_certificate"
)

const (
	AttributeKeySubmitter       = "submitter"
	AttributeKeyPoolID          = "pool_id"
	AttributeKeyIntentID        = "intent_id"
	AttributeKeyResponseID      = "response_id"
	AttributeKeyFinalizeID      = "finalize_id"
	AttributeKeyCertificateHash = "certificate_hash"
	AttributeKeyReplayKey       = "replay_key"
	AttributeKeyMatchID         = "match_id"
)
