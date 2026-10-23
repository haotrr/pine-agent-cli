//go:build darwin && cgo

package engine

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <Security/Security.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>
#include <string.h>

static CFMutableDictionaryRef pine_query(const char *account) {
 CFMutableDictionaryRef q = CFDictionaryCreateMutable(NULL,0,&kCFTypeDictionaryKeyCallBacks,&kCFTypeDictionaryValueCallBacks);
 CFStringRef a = CFStringCreateWithCString(NULL,account,kCFStringEncodingUTF8);
 CFDictionarySetValue(q,kSecClass,kSecClassGenericPassword);
 CFDictionarySetValue(q,kSecAttrService,CFSTR("pine-agent-cli.auth.v1"));
 CFDictionarySetValue(q,kSecAttrAccount,a);
 CFDictionarySetValue(q,kSecAttrSynchronizable,kCFBooleanFalse);
 CFRelease(a);
 return q;
}
static OSStatus pine_read(const char *account, void **out, long *size) {
 CFMutableDictionaryRef q = pine_query(account);
 CFDictionarySetValue(q,kSecReturnData,kCFBooleanTrue);
 CFDictionarySetValue(q,kSecMatchLimit,kSecMatchLimitOne);
 CFTypeRef result = NULL;
 OSStatus s = SecItemCopyMatching(q,&result);
 CFRelease(q);
 if(s == errSecSuccess) {
  if(!result || CFGetTypeID(result)!=CFDataGetTypeID()) {if(result)CFRelease(result);return errSecDecode;}
  *size=CFDataGetLength((CFDataRef)result);
  if(*size<1 || *size>65536) {CFRelease(result);return errSecDecode;}
  *out=malloc(*size);
  if(!*out) {CFRelease(result);return errSecAllocate;}
  memcpy(*out,CFDataGetBytePtr((CFDataRef)result),*size);
  CFRelease(result);
 }
 return s;
}
static OSStatus pine_add(const char *account,const void *data,long size) {
 CFMutableDictionaryRef q = pine_query(account);
 CFDataRef d = CFDataCreate(NULL,data,size);
 CFDictionarySetValue(q,kSecValueData,d);
 CFDictionarySetValue(q,kSecAttrAccessible,kSecAttrAccessibleWhenUnlockedThisDeviceOnly);
 OSStatus s = SecItemAdd(q,NULL);
 CFRelease(d);CFRelease(q);
 return s;
}
static void pine_clear(void *p, long size) { if(p){memset_s(p,size,0,size);free(p);} }
*/
import "C"
import "unsafe"

type keychainStore struct{}

func NativeAuthStore() AuthStore { return keychainStore{} }
func (keychainStore) Get(account string) ([]byte, error) {
	a := C.CString(account)
	defer C.free(unsafe.Pointer(a))
	var p unsafe.Pointer
	var n C.long
	status := C.pine_read(a, &p, &n)
	if status == C.errSecItemNotFound {
		return nil, ErrAuthMissing
	}
	if status != C.errSecSuccess {
		return nil, Fail("keychain", "Keychain read denied or unavailable", 4)
	}
	defer C.pine_clear(p, n)
	return C.GoBytes(p, C.int(n)), nil
}
func (keychainStore) Add(account string, b []byte) error {
	a := C.CString(account)
	defer C.free(unsafe.Pointer(a))
	p := C.CBytes(b)
	defer C.pine_clear(p, C.long(len(b)))
	if C.pine_add(a, p, C.long(len(b))) != C.errSecSuccess {
		return Fail("keychain", "Keychain save denied or record already exists", 4)
	}
	return nil
}
