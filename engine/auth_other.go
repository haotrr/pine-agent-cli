//go:build !darwin || !cgo

package engine

type unavailableAuthStore struct{}

func NativeAuthStore() AuthStore { return unavailableAuthStore{} }
func (unavailableAuthStore) Get(string) ([]byte, error) {
	return nil, Fail("keychain_unavailable", "Keychain requires macOS with a cgo build; use environment credentials", 4)
}
func (unavailableAuthStore) Add(string, []byte) error {
	return Fail("keychain_unavailable", "Keychain requires macOS with a cgo build; use environment credentials", 4)
}
