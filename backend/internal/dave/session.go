// Adapted from github.com/disgoorg/godave/golibdave v0.3.0; see LICENSE and PROVENANCE.md.
package dave

import (
	"errors"
	"log/slog"
	"sync"

	"github.com/disgoorg/godave"
	"github.com/disgoorg/godave/libdave"
)

const (
	initTransitionId         = 0
	disabledProtocolVersion  = 0
	mlsNewGroupExpectedEpoch = 1
)

var (
	_ godave.SessionCreateFunc = NewSession
	_ godave.Session           = (*session)(nil)
)

// NewSession returns a new DAVE session using libdave.
func NewSession(logger *slog.Logger, selfUserID godave.UserID, callbacks godave.Callbacks) godave.Session {
	if logger == nil {
		logger = slog.Default()
	}
	encryptor := libdave.NewEncryptor()
	// Start in Passthrough by default
	encryptor.SetPassthroughMode(true)

	return &session{
		selfUserID: selfUserID,
		callbacks:  callbacks,
		logger:     logger,
		// Context and authSessionID are only used with persistent key storage and can be ignored most of the time
		session:             libdave.NewSession("", ""),
		encryptor:           encryptor,
		decryptors:          make(map[godave.UserID]*libdave.Decryptor),
		preparedTransitions: make(map[uint16]uint16),
	}
}

type session struct {
	mu                            sync.Mutex
	closed                        bool
	negotiated                    bool
	activeVersion                 uint16
	selfUserID                    godave.UserID
	channelID                     godave.ChannelID
	logger                        *slog.Logger
	callbacks                     godave.Callbacks
	session                       *libdave.Session
	encryptor                     *libdave.Encryptor
	decryptors                    map[godave.UserID]*libdave.Decryptor
	preparedTransitions           map[uint16]uint16
	lastPreparedTransitionVersion uint16
}

func (s *session) MaxSupportedProtocolVersion() int {
	return int(libdave.MaxSupportedProtocolVersion())
}

func (s *session) Ready() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ready()
}

func (s *session) ready() bool {
	return !s.closed && s.negotiated && (s.activeVersion == 0 || (!s.encryptor.IsPassthroughMode() && s.encryptor.HasKeyRatchet()))
}

// libdave's Go binding owns native destruction through finalizers. Stop admission
// first and release references; no media call can race native object retirement.
func (s *session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.session = nil
	s.encryptor = nil
	clear(s.decryptors)
	clear(s.preparedTransitions)
	return nil
}

func (s *session) SetChannelID(channelID godave.ChannelID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.channelID = channelID
}

func (s *session) AssignSsrcToCodec(ssrc uint32, codec godave.Codec) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.encryptor.AssignSsrcToCodec(ssrc, libdave.Codec(codec))
}

func (s *session) MaxEncryptedFrameSize(frameSize int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0
	}
	return s.encryptor.GetMaxCiphertextByteSize(libdave.MediaTypeAudio, frameSize)
}

func (s *session) Encrypt(ssrc uint32, frame []byte, encryptedFrame []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.encrypt(libdave.MediaTypeAudio, ssrc, frame, encryptedFrame)
}

func (s *session) MaxDecryptedFrameSize(userID godave.UserID, frameSize int) int {
	// DAVE removes authenticated framing; plaintext cannot exceed ciphertext.
	return frameSize
}

func (s *session) Decrypt(userID godave.UserID, frame []byte, decryptedFrame []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.decrypt(libdave.MediaTypeAudio, userID, frame, decryptedFrame)
}

func (s *session) AssignH264(ssrc uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.encryptor.AssignSsrcToCodec(ssrc, libdave.CodecH264)
	}
}

func (s *session) EncryptVideo(ssrc uint32, frame []byte) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ready() {
		return nil, errors.New("DAVE epoch is not ready")
	}
	out := make([]byte, s.encryptor.GetMaxCiphertextByteSize(libdave.MediaTypeVideo, len(frame)))
	n, err := s.encrypt(libdave.MediaTypeVideo, ssrc, frame, out)
	if err != nil {
		return nil, err
	}
	return out[:n], nil
}

func (s *session) DecryptVideo(userID godave.UserID, frame, out []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.decrypt(libdave.MediaTypeVideo, userID, frame, out)
}

func (s *session) encrypt(kind libdave.MediaType, ssrc uint32, frame, out []byte) (int, error) {
	if !s.ready() {
		return 0, errors.New("DAVE epoch is not ready")
	}
	if len(frame) == 0 || len(out) < s.encryptor.GetMaxCiphertextByteSize(kind, len(frame)) {
		return 0, errors.New("invalid DAVE frame buffer")
	}
	return s.encryptor.Encrypt(kind, ssrc, frame, out)
}

func (s *session) decrypt(kind libdave.MediaType, user godave.UserID, frame, out []byte) (int, error) {
	if !s.ready() {
		return 0, errors.New("DAVE epoch is not ready")
	}
	d := s.decryptors[user]
	if d == nil {
		return 0, errors.New("unknown DAVE participant")
	}
	if len(frame) == 0 || len(out) < d.GetMaxPlaintextByteSize(kind, len(frame)) {
		return 0, errors.New("invalid DAVE frame buffer")
	}
	return d.Decrypt(kind, frame, out)
}

func (s *session) AddUser(userID godave.UserID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if userID == s.selfUserID || s.decryptors[userID] != nil || len(s.decryptors) >= 512 {
		return
	}
	s.decryptors[userID] = libdave.NewDecryptor()
	s.setupKeyRatchetForUser(userID, s.lastPreparedTransitionVersion)
}

func (s *session) RemoveUser(userID godave.UserID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	delete(s.decryptors, userID)
}

func (s *session) OnSelectProtocolAck(protocolVersion uint16) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.protocolInit(protocolVersion)
}

func (s *session) OnDavePrepareTransition(transitionID uint16, protocolVersion uint16) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.prepareTransition(transitionID, protocolVersion)

	if transitionID != initTransitionId {
		s.sendReadyForTransition(transitionID)
	}
}

func (s *session) OnDaveExecuteTransition(transitionID uint16) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.executeTransition(transitionID)
}

func (s *session) OnDavePrepareEpoch(epoch int, protocolVersion uint16) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.prepareEpoch(epoch, protocolVersion)

	if epoch == mlsNewGroupExpectedEpoch {
		s.sendMLSKeyPackage()
	}
}

func (s *session) OnDaveMLSExternalSenderPackage(externalSenderPackage []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if len(externalSenderPackage) == 0 {
		return
	}
	s.session.SetExternalSender(externalSenderPackage)
}

func (s *session) OnDaveMLSProposals(proposals []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if len(proposals) == 0 {
		return
	}
	commitWelcome := s.session.ProcessProposals(proposals, s.recognizedUserIDs())

	if commitWelcome != nil {
		s.sendMLSCommitWelcome(commitWelcome)
	}
}

func (s *session) OnDaveMLSPrepareCommitTransition(transitionID uint16, commitMessage []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if len(commitMessage) == 0 {
		return
	}
	res := s.session.ProcessCommit(commitMessage)

	if res.IsIgnored() {
		return
	}

	if res.IsFailed() {
		s.sendInvalidCommitWelcome(transitionID)
		s.protocolInit(s.session.GetProtocolVersion())
		return
	}

	s.prepareTransition(transitionID, s.session.GetProtocolVersion())
	if transitionID != initTransitionId {
		s.sendReadyForTransition(transitionID)
	}
}

func (s *session) OnDaveMLSWelcome(transitionID uint16, welcomeMessage []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if len(welcomeMessage) == 0 {
		return
	}
	res := s.session.ProcessWelcome(welcomeMessage, s.recognizedUserIDs())

	if res == nil {
		s.sendInvalidCommitWelcome(transitionID)
		s.sendMLSKeyPackage()
		return
	}

	s.prepareTransition(transitionID, s.session.GetProtocolVersion())
	if transitionID != initTransitionId {
		s.sendReadyForTransition(transitionID)
	}
}

func (s *session) recognizedUserIDs() []string {
	userIDs := make([]string, 0, len(s.decryptors)+1)

	userIDs = append(userIDs, string(s.selfUserID))

	for userID := range s.decryptors {
		userIDs = append(userIDs, string(userID))
	}

	return userIDs
}

func (s *session) protocolInit(protocolVersion uint16) {
	if protocolVersion > disabledProtocolVersion {
		s.prepareEpoch(mlsNewGroupExpectedEpoch, protocolVersion)
		s.sendMLSKeyPackage()
	} else {
		s.prepareTransition(initTransitionId, protocolVersion)
		s.executeTransition(initTransitionId)
	}
}

func (s *session) prepareEpoch(epoch int, protocolVersion uint16) {
	if epoch != mlsNewGroupExpectedEpoch {
		return
	}

	s.negotiated = false
	s.session.Init(protocolVersion, uint64(s.channelID), string(s.selfUserID))
}

func (s *session) executeTransition(transitionID uint16) {
	protocolVersion, ok := s.preparedTransitions[transitionID]
	if !ok {
		return
	}

	delete(s.preparedTransitions, transitionID)

	if protocolVersion == disabledProtocolVersion {
		s.session.Reset()
	}

	s.setupKeyRatchetForUser(s.selfUserID, protocolVersion)
}

func (s *session) prepareTransition(transitionID uint16, protocolVersion uint16) {
	for userID := range s.decryptors {
		s.setupKeyRatchetForUser(userID, protocolVersion)
	}

	if transitionID == initTransitionId {
		s.setupKeyRatchetForUser(s.selfUserID, protocolVersion)
	} else {
		if len(s.preparedTransitions) >= 32 {
			clear(s.preparedTransitions)
		}
		s.preparedTransitions[transitionID] = protocolVersion
	}

	s.lastPreparedTransitionVersion = protocolVersion
}

func (s *session) setupKeyRatchetForUser(userID godave.UserID, protocolVersion uint16) {
	disabled := protocolVersion == disabledProtocolVersion

	if userID == s.selfUserID {
		s.activeVersion = protocolVersion
		s.negotiated = true
		s.encryptor.SetPassthroughMode(disabled)
		if !disabled {
			s.encryptor.SetKeyRatchet(s.session.GetKeyRatchet(string(userID)))
		}
		return
	}

	decryptor := s.decryptors[userID]
	decryptor.TransitionToPassthroughMode(disabled)
	if !disabled {
		decryptor.TransitionToKeyRatchet(s.session.GetKeyRatchet(string(userID)))
	}
}

func (s *session) sendMLSKeyPackage() {
	if err := s.callbacks.SendMLSKeyPackage(s.session.GetMarshalledKeyPackage()); err != nil {
		s.logger.Error("failed to send MLS key package", slog.Any("err", err))
	}
}

func (s *session) sendMLSCommitWelcome(message []byte) {
	if err := s.callbacks.SendMLSCommitWelcome(message); err != nil {
		s.logger.Error("failed to send MLS commit welcome", slog.Any("err", err))
	}
}

func (s *session) sendReadyForTransition(transitionID uint16) {
	if err := s.callbacks.SendReadyForTransition(transitionID); err != nil {
		s.logger.Error("failed to send ready for transition", slog.Any("err", err))
	}
}

func (s *session) sendInvalidCommitWelcome(transitionID uint16) {
	if err := s.callbacks.SendInvalidCommitWelcome(transitionID); err != nil {
		s.logger.Error("failed to send invalid commit welcome", slog.Any("err", err))
	}
}
