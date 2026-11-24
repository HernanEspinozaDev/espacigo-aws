package spaces

// AddressHasContact rejects addresses that embed contact details in text sent
// to an external geocoding provider.
func AddressHasContact(address string) bool { return commercialAddressContact.MatchString(address) }
