//go:build libdave

// Package davevideo provides an opt-in official libdave media session. It is
// not selected by the current voice engine or a screen-sharing UI.
package davevideo

/*
#cgo pkg-config: dave
#include <dave/dave.h>
#include <stdlib.h>
*/
import "C"

import (
	"crypto/sha256"
	"errors"
	"log/slog"
	"sync"
	"unsafe"

	"github.com/disgoorg/godave"
)

var _ godave.Session = (*Session)(nil)

type Session struct {
	mu             sync.Mutex
	native         C.DAVESessionHandle
	encryptor      C.DAVEEncryptorHandle
	decryptors     map[godave.UserID]C.DAVEDecryptorHandle
	transitions    map[uint16]uint16
	self           godave.UserID
	channel        godave.ChannelID
	callbacks      godave.Callbacks
	closed         bool
	installedEpoch [32]byte
	failed         bool
}

func New(_ *slog.Logger, self godave.UserID, callbacks godave.Callbacks) godave.Session {
	return &Session{native: C.daveSessionCreate(nil, nil, nil, nil), encryptor: C.daveEncryptorCreate(), decryptors: make(map[godave.UserID]C.DAVEDecryptorHandle), transitions: make(map[uint16]uint16), self: self, callbacks: callbacks}
}
func pointer(b []byte) *C.uint8_t {
	if len(b) == 0 {
		return nil
	}
	return (*C.uint8_t)(unsafe.Pointer(&b[0]))
}
func (s *Session) MaxSupportedProtocolVersion() int { return int(C.daveMaxSupportedProtocolVersion()) }
func (s *Session) Ready() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.closed && !s.failed && s.encryptor != nil && bool(C.daveEncryptorHasKeyRatchet(s.encryptor)) && !bool(C.daveEncryptorIsPassthroughMode(s.encryptor))
}
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	for _, d := range s.decryptors {
		C.daveDecryptorDestroy(d)
	}
	s.decryptors = nil
	if s.encryptor != nil {
		C.daveEncryptorDestroy(s.encryptor)
		s.encryptor = nil
	}
	if s.native != nil {
		C.daveSessionDestroy(s.native)
		s.native = nil
	}
	return nil
}
func (s *Session) SetChannelID(id godave.ChannelID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.channel != id {
		s.failed = true
		s.transitions = make(map[uint16]uint16)
	}
	s.channel = id
}
func (s *Session) AssignSsrcToCodec(ssrc uint32, codec godave.Codec) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		C.daveEncryptorAssignSsrcToCodec(s.encryptor, C.uint32_t(ssrc), C.DAVECodec(codec))
	}
}
func (s *Session) MaxEncryptedFrameSize(n int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || n < 0 || n > 4<<20 {
		return 0
	}
	return int(C.daveEncryptorGetMaxCiphertextByteSize(s.encryptor, C.DAVE_MEDIA_TYPE_AUDIO, C.size_t(n)))
}
func (s *Session) MaxDecryptedFrameSize(_ godave.UserID, n int) int { return n }
func (s *Session) encryptLocked(media C.DAVEMediaType, ssrc uint32, in, out []byte) (int, error) {
	if s.closed || s.failed || s.encryptor == nil || !bool(C.daveEncryptorHasKeyRatchet(s.encryptor)) || bool(C.daveEncryptorIsPassthroughMode(s.encryptor)) {
		return 0, errors.New("DAVE media epoch unavailable")
	}
	if len(in) == 0 || len(in) > 4<<20 || len(out) == 0 {
		return 0, errors.New("invalid media frame")
	}
	var n C.size_t
	result := C.daveEncryptorEncrypt(s.encryptor, media, C.uint32_t(ssrc), pointer(in), C.size_t(len(in)), pointer(out), C.size_t(len(out)), &n)
	if result != C.DAVE_ENCRYPTOR_RESULT_CODE_SUCCESS || int(n) > len(out) {
		return 0, errors.New("libdave media encryption failed")
	}
	return int(n), nil
}
func (s *Session) Encrypt(ssrc uint32, in, out []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.encryptLocked(C.DAVE_MEDIA_TYPE_AUDIO, ssrc, in, out)
}
func (s *Session) EncryptH264(ssrc uint32, in []byte) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || len(in) == 0 || len(in) > 4<<20 {
		return nil, errors.New("invalid video frame or closed session")
	}
	C.daveEncryptorAssignSsrcToCodec(s.encryptor, C.uint32_t(ssrc), C.DAVE_CODEC_H264)
	cap := int(C.daveEncryptorGetMaxCiphertextByteSize(s.encryptor, C.DAVE_MEDIA_TYPE_VIDEO, C.size_t(len(in))))
	if cap <= 0 || cap > 8<<20 {
		return nil, errors.New("video ciphertext budget exceeded")
	}
	out := make([]byte, cap)
	n, err := s.encryptLocked(C.DAVE_MEDIA_TYPE_VIDEO, ssrc, in, out)
	if err != nil {
		return nil, err
	}
	return out[:n], nil
}
func (s *Session) Decrypt(user godave.UserID, in, out []byte) (int, error) {
	return s.decrypt(user, in, out, C.DAVE_MEDIA_TYPE_AUDIO)
}
func (s *Session) DecryptVideo(user godave.UserID, in []byte) ([]byte, error) {
	if len(in) == 0 || len(in) > 8<<20 {
		return nil, errors.New("invalid video ciphertext size")
	}
	out := make([]byte, len(in))
	n, err := s.decrypt(user, in, out, C.DAVE_MEDIA_TYPE_VIDEO)
	return out[:n], err
}
func (s *Session) decrypt(user godave.UserID, in, out []byte, media C.DAVEMediaType) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.decryptors[user]
	if s.closed || d == nil || len(in) == 0 || len(in) > 8<<20 || len(out) == 0 {
		return 0, errors.New("DAVE decryptor unavailable")
	}
	var n C.size_t
	result := C.daveDecryptorDecrypt(d, media, pointer(in), C.size_t(len(in)), pointer(out), C.size_t(len(out)), &n)
	if result != C.DAVE_DECRYPTOR_RESULT_CODE_SUCCESS || int(n) > len(out) {
		return 0, errors.New("libdave authentication failed")
	}
	return int(n), nil
}
func (s *Session) ratchetLocked(user godave.UserID) {
	if s.closed {
		return
	}
	name := C.CString(string(user))
	defer C.free(unsafe.Pointer(name))
	r := C.daveSessionGetKeyRatchet(s.native, name)
	if r == nil {
		return
	}
	defer C.daveKeyRatchetDestroy(r)
	if user == s.self {
		var auth *C.uint8_t
		var length C.size_t
		C.daveSessionGetLastEpochAuthenticator(s.native, &auth, &length)
		if auth == nil || length == 0 || length > 1024 {
			if auth != nil {
				C.daveFree(unsafe.Pointer(auth))
			}
			s.failed = true
			return
		}
		epoch := sha256.Sum256(C.GoBytes(unsafe.Pointer(auth), C.int(length)))
		C.daveFree(unsafe.Pointer(auth))
		// SetKeyRatchet resets the native nonce. Repeating an event in the same
		// epoch must not reinstall its key and restart that nonce at zero.
		if epoch != s.installedEpoch {
			C.daveEncryptorSetPassthroughMode(s.encryptor, false)
			C.daveEncryptorSetKeyRatchet(s.encryptor, r)
			s.installedEpoch = epoch
		}
	}
	if d := s.decryptors[user]; d != nil {
		C.daveDecryptorTransitionToPassthroughMode(d, false)
		C.daveDecryptorTransitionToKeyRatchet(d, r)
	}
}
func (s *Session) AddUser(id godave.UserID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.decryptors[id] != nil || len(s.decryptors) >= 512 {
		return
	}
	s.decryptors[id] = C.daveDecryptorCreate()
	s.ratchetLocked(id)
}
func (s *Session) RemoveUser(id godave.UserID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d := s.decryptors[id]; d != nil {
		C.daveDecryptorDestroy(d)
		delete(s.decryptors, id)
	}
}
func (s *Session) initLocked(version uint16) {
	if s.closed {
		return
	}
	s.failed = false
	s.transitions = make(map[uint16]uint16)
	s.installedEpoch = [32]byte{}
	for id, d := range s.decryptors {
		C.daveDecryptorDestroy(d)
		s.decryptors[id] = C.daveDecryptorCreate()
	}
	if version == 0 || version > uint16(C.daveMaxSupportedProtocolVersion()) || s.channel == 0 {
		s.failed = true
		return
	}
	name := C.CString(string(s.self))
	defer C.free(unsafe.Pointer(name))
	C.daveSessionInit(s.native, C.uint16_t(version), C.uint64_t(s.channel), name)
	// Never preserve an old sending ratchet while a fresh group is negotiating.
	C.daveEncryptorDestroy(s.encryptor)
	s.encryptor = C.daveEncryptorCreate()
}
func (s *Session) keyPackageLocked() []byte {
	var p *C.uint8_t
	var n C.size_t
	C.daveSessionGetMarshalledKeyPackage(s.native, &p, &n)
	if p == nil {
		return nil
	}
	defer C.daveFree(unsafe.Pointer(p))
	if n > 1<<20 {
		return nil
	}
	return C.GoBytes(unsafe.Pointer(p), C.int(n))
}
func (s *Session) OnSelectProtocolAck(v uint16) { s.OnDavePrepareEpoch(1, v) }
func (s *Session) OnDavePrepareEpoch(epoch int, v uint16) {
	s.mu.Lock()
	if s.closed || epoch != 1 {
		s.mu.Unlock()
		return
	}
	s.initLocked(v)
	var key []byte
	if v > 0 && !s.failed {
		key = s.keyPackageLocked()
	}
	s.mu.Unlock()
	if len(key) > 0 && s.callbacks != nil {
		if err := s.callbacks.SendMLSKeyPackage(key); err != nil {
			s.markFailed()
		}
	}
}
func (s *Session) markFailed() { s.mu.Lock(); s.failed = true; s.mu.Unlock() }
func (s *Session) prepareLocked(id, version uint16) {
	if version == 0 {
		s.failed = true
		return
	}
	for user := range s.decryptors {
		s.ratchetLocked(user)
	}
	if id == 0 {
		s.ratchetLocked(s.self)
	} else {
		if _, known := s.transitions[id]; !known && len(s.transitions) >= 32 {
			s.failed = true
			return
		}
		s.transitions[id] = version
	}
}
func (s *Session) OnDavePrepareTransition(id, v uint16) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.prepareLocked(id, v)
	failed := s.failed
	s.mu.Unlock()
	if !failed && id != 0 && s.callbacks != nil {
		if err := s.callbacks.SendReadyForTransition(id); err != nil {
			s.markFailed()
		}
	}
}
func (s *Session) OnDaveExecuteTransition(id uint16) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if v, ok := s.transitions[id]; ok {
		delete(s.transitions, id)
		if v > 0 {
			s.ratchetLocked(s.self)
		} else {
			s.failed = true
		}
	}
}
func (s *Session) OnDaveMLSExternalSenderPackage(b []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed && len(b) > 0 && len(b) <= 1<<20 {
		C.daveSessionSetExternalSender(s.native, pointer(b), C.size_t(len(b)))
	}
}
func (s *Session) recognizedLocked() ([]*C.char, func()) {
	names := make([]*C.char, 0, len(s.decryptors)+1)
	names = append(names, C.CString(string(s.self)))
	for id := range s.decryptors {
		if id != s.self {
			names = append(names, C.CString(string(id)))
		}
	}
	return names, func() {
		for _, p := range names {
			C.free(unsafe.Pointer(p))
		}
	}
}
func (s *Session) OnDaveMLSProposals(b []byte) {
	s.mu.Lock()
	if s.closed || len(b) == 0 || len(b) > 1<<20 {
		s.mu.Unlock()
		return
	}
	names, free := s.recognizedLocked()
	var out *C.uint8_t
	var n C.size_t
	C.daveSessionProcessProposals(s.native, pointer(b), C.size_t(len(b)), (**C.char)(unsafe.Pointer(&names[0])), C.size_t(len(names)), &out, &n)
	free()
	var message []byte
	if out != nil {
		if n <= 1<<20 {
			message = C.GoBytes(unsafe.Pointer(out), C.int(n))
		}
		C.daveFree(unsafe.Pointer(out))
	}
	s.mu.Unlock()
	if len(message) > 0 && s.callbacks != nil {
		if err := s.callbacks.SendMLSCommitWelcome(message); err != nil {
			s.markFailed()
		}
	}
}
func (s *Session) OnDaveMLSPrepareCommitTransition(id uint16, b []byte) {
	s.mu.Lock()
	if s.closed || len(b) == 0 || len(b) > 1<<20 {
		s.mu.Unlock()
		return
	}
	result := C.daveSessionProcessCommit(s.native, pointer(b), C.size_t(len(b)))
	ok := result != nil && !bool(C.daveCommitResultIsFailed(result))
	ignored := result != nil && bool(C.daveCommitResultIsIgnored(result))
	if result != nil {
		C.daveCommitResultDestroy(result)
	}
	if ok && !ignored {
		s.prepareLocked(id, uint16(C.daveSessionGetProtocolVersion(s.native)))
		ok = !s.failed
	} else if !ok {
		s.failed = true
	}
	s.mu.Unlock()
	if s.callbacks != nil {
		if !ok {
			if err := s.callbacks.SendInvalidCommitWelcome(id); err != nil {
				s.markFailed()
			}
		} else if !ignored && id != 0 {
			if err := s.callbacks.SendReadyForTransition(id); err != nil {
				s.markFailed()
			}
		}
	}
}
func (s *Session) OnDaveMLSWelcome(id uint16, b []byte) {
	s.mu.Lock()
	if s.closed || len(b) == 0 || len(b) > 1<<20 {
		s.mu.Unlock()
		return
	}
	names, free := s.recognizedLocked()
	result := C.daveSessionProcessWelcome(s.native, pointer(b), C.size_t(len(b)), (**C.char)(unsafe.Pointer(&names[0])), C.size_t(len(names)))
	free()
	ok := result != nil
	if result != nil {
		C.daveWelcomeResultDestroy(result)
		s.prepareLocked(id, uint16(C.daveSessionGetProtocolVersion(s.native)))
		ok = !s.failed
	} else {
		s.failed = true
	}
	s.mu.Unlock()
	if s.callbacks != nil {
		if !ok {
			if err := s.callbacks.SendInvalidCommitWelcome(id); err != nil {
				s.markFailed()
			}
		} else if id != 0 {
			if err := s.callbacks.SendReadyForTransition(id); err != nil {
				s.markFailed()
			}
		}
	}
}
