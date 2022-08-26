### cryptopan - Go implementation of Crypto-PAn
#### Yawning Angel (yawning at schwanenlied dot me)

Package cryptopan implements the Crypto-PAn prefix-preserving IP address
sanitization algorithm as specified by J. Fan, J. Xu, M. Ammar, and S. Moon.

Crypto-PAn has the following properties:

 * One-to-one - The mapping from original IP addresses to anonymized IP
   addresses is one-to-one.

 * Prefix-preserving - In Crypto-PAn, the IP address anonymization is
   prefix-preserving. That is, if two original IP addresses share a k-bit
   prefix, their anonymized mappings will also share a k-bit prefix.

 * Consistent across traces - Crypto-PAn allows multiple traces to be
   sanitized in a consistent way, over time and across locations.  That is,
   the same IP address in different traces is anonymized to the same
   address, even though the traces might be sanitized separately at
   different time and/or at different locations.

 * Cryptography-based - To sanitize traces, trace owners provide Crypto-PAn
   a secret key.  Anonymization consistency across multiple traces is
   achieved by the use of the same key.  The construction of Crypto-PAn
   preserves the secrecy of the key and the (pseudo)randomness of the
   mapping from an original IP address to its anonymized counterpart.

As an experimental extension, anonymizing IPv6 addresses is also somewhat
supported, but is untested beyond a cursory examination of the output.
