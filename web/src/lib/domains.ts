// DNS verification helpers for the custom-domain settings UI (#122). The label
// prefix mirrors the backend's proof-of-control convention (ADR-0004): the owner
// publishes a TXT record at _yaadegar-verify.<hostname> carrying the token the
// AddDomain response returned.
export const TXT_VERIFY_PREFIX = '_yaadegar-verify';

// txtRecordName is the DNS name at which the verification TXT record must be
// published for a given custom hostname.
export function txtRecordName(hostname: string): string {
	return `${TXT_VERIFY_PREFIX}.${hostname}`;
}
