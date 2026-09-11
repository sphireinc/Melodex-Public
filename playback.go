package main

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/faiface/beep"
	"github.com/faiface/beep/effects"
	"github.com/faiface/beep/flac"
	"github.com/faiface/beep/mp3"
	"github.com/faiface/beep/speaker"
	"github.com/faiface/beep/vorbis"
	"github.com/faiface/beep/wav"
)

type playbackEngine struct {
	mu               sync.Mutex
	lookup           map[string]TrackRecord
	queue            []string
	queueSource      string
	current          *activePlayback
	speakerReady     bool
	speakerRate      beep.SampleRate
	volume           float64
	muted            bool
	seekSilenceTimer *time.Timer
	shuffleEnabled   bool
	repeatMode       string
	lastError        string
}

type activePlayback struct {
	track         TrackRecord
	queue         []string
	queueSource   string
	file          *os.File
	streamer      beep.StreamSeekCloser
	ctrl          *beep.Ctrl
	volumeControl *effects.Volume
	sampleRate    beep.SampleRate
	samples       int
	startedAt     time.Time
	pausedAt      int
	playing       bool
	loading       bool
}

func newPlaybackEngine() *playbackEngine {
	return &playbackEngine{
		lookup:     map[string]TrackRecord{},
		volume:     0.84,
		repeatMode: "off",
	}
}

func (p *playbackEngine) setLookup(tracks map[string]TrackRecord) {
	p.mu.Lock()
	defer p.mu.Unlock()
	next := make(map[string]TrackRecord, len(tracks))
	for id, track := range tracks {
		next[id] = track
	}
	p.lookup = next
}

func (p *playbackEngine) snapshot() PlaybackState {
	p.mu.Lock()
	defer p.mu.Unlock()
	state := PlaybackState{
		Volume:         p.volume,
		Muted:          p.muted,
		ShuffleEnabled: p.shuffleEnabled,
		RepeatMode:     p.repeatMode,
		Error:          p.lastError,
	}
	state.Queue = append([]string{}, p.queue...)
	state.QueueSource = p.queueSource
	if p.current == nil {
		return state
	}
	current := p.current
	state.CurrentTrackID = current.track.ID
	state.CurrentTrackPath = current.track.AudioPath
	state.IsPlaying = current.playing
	state.IsLoading = current.loading
	state.Duration = current.durationSeconds()
	state.CurrentTime = current.currentSeconds()
	state.Volume = p.volume
	state.Muted = p.muted
	state.ShuffleEnabled = p.shuffleEnabled
	state.RepeatMode = p.repeatMode
	state.Error = p.lastError
	return state
}

func (p *playbackEngine) play(trackID string, queueIDs []string, queueSource string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	track, ok := p.lookup[trackID]
	if !ok {
		logEvent("playback_play_failed", "track_id", trackID, "queue_source", queueSource, "reason", "track_not_found")
		return fmt.Errorf("track %s not found", trackID)
	}
	queueIDs = normalizeQueue(queueIDs, trackID)
	if p.shuffleEnabled && len(queueIDs) > 1 && queueSource != "manual" {
		queueIDs = shuffleQueue(queueIDs, trackID)
	}
	p.queue = append([]string(nil), queueIDs...)
	p.queueSource = queueSource
	logEvent("playback_play_started", "track_id", trackID, "queue_source", queueSource, "queue_length", len(queueIDs))
	return p.startLocked(track, queueIDs, queueSource, true)
}

func (p *playbackEngine) togglePlayback() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current == nil {
		if len(p.queue) == 0 {
			logEvent("playback_toggle_failed", "reason", "no_track_selected")
			return errors.New("no track selected")
		}
		track, ok := p.lookup[p.queue[0]]
		if !ok {
			logEvent("playback_toggle_failed", "track_id", p.queue[0], "reason", "track_not_found")
			return fmt.Errorf("track %s not found", p.queue[0])
		}
		return p.startLocked(track, p.queue, p.queueSource, true)
	}
	if p.current.playing {
		logEvent("playback_paused", "track_id", p.current.track.ID, "queue_source", p.queueSource)
		p.current.pauseLocked()
		return nil
	}
	logEvent("playback_resumed", "track_id", p.current.track.ID, "queue_source", p.queueSource)
	p.current.resumeLocked()
	return nil
}

func (p *playbackEngine) pause() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current == nil {
		logEvent("playback_pause_failed", "reason", "no_track_selected")
		return errors.New("no track selected")
	}
	if !p.current.playing {
		return nil
	}
	logEvent("playback_paused", "track_id", p.current.track.ID, "queue_source", p.queueSource)
	p.current.pauseLocked()
	return nil
}

func (p *playbackEngine) next() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current == nil {
		if len(p.queue) == 0 {
			logEvent("playback_next_failed", "reason", "no_track_selected")
			return errors.New("no track selected")
		}
		track, ok := p.lookup[p.queue[0]]
		if !ok {
			logEvent("playback_next_failed", "track_id", p.queue[0], "reason", "track_not_found")
			return fmt.Errorf("track %s not found", p.queue[0])
		}
		return p.startLocked(track, p.queue, p.queueSource, true)
	}
	nextID := p.nextTrackIDLocked()
	if nextID == "" {
		logEvent("playback_next_reached_end", "track_id", p.current.track.ID, "repeat_mode", p.repeatMode)
		return nil
	}
	track, ok := p.lookup[nextID]
	if !ok {
		logEvent("playback_next_failed", "track_id", nextID, "reason", "track_not_found")
		return fmt.Errorf("track %s not found", nextID)
	}
	logEvent("playback_next", "from_track_id", p.current.track.ID, "to_track_id", nextID, "queue_source", p.queueSource)
	return p.startLocked(track, p.queue, p.queueSource, true)
}

func (p *playbackEngine) previous() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current == nil {
		if len(p.queue) == 0 {
			logEvent("playback_previous_failed", "reason", "no_track_selected")
			return errors.New("no track selected")
		}
		track, ok := p.lookup[p.queue[0]]
		if !ok {
			logEvent("playback_previous_failed", "track_id", p.queue[0], "reason", "track_not_found")
			return fmt.Errorf("track %s not found", p.queue[0])
		}
		return p.startLocked(track, p.queue, p.queueSource, true)
	}
	prevID := p.previousTrackIDLocked()
	if prevID == "" {
		logEvent("playback_previous_reached_start", "track_id", p.current.track.ID, "repeat_mode", p.repeatMode)
		return nil
	}
	track, ok := p.lookup[prevID]
	if !ok {
		logEvent("playback_previous_failed", "track_id", prevID, "reason", "track_not_found")
		return fmt.Errorf("track %s not found", prevID)
	}
	logEvent("playback_previous", "from_track_id", p.current.track.ID, "to_track_id", prevID, "queue_source", p.queueSource)
	return p.startLocked(track, p.queue, p.queueSource, true)
}

func (p *playbackEngine) seek(ratio float64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current == nil {
		logEvent("playback_seek_failed", "reason", "no_track_selected", "ratio", ratio)
		return errors.New("no track selected")
	}
	ratio = clampFloat(ratio, 0, 1)
	position := int(float64(p.current.samples) * ratio)
	logEvent("playback_seek_requested", "track_id", p.current.track.ID, "ratio", ratio, "sample_position", position)
	speaker.Lock()
	p.silenceSeekLockedNoLock()
	if err := p.current.streamer.Seek(position); err != nil {
		logEvent("playback_seek_failed", "track_id", p.current.track.ID, "ratio", ratio, "error", err.Error())
		p.restoreVolumeLockedNoLock()
		speaker.Unlock()
		return err
	}
	p.current.pausedAt = position
	if p.current.playing {
		p.current.startedAt = time.Now().UTC()
	}
	speaker.Unlock()
	p.scheduleSeekVolumeRestoreLocked()
	return nil
}

func (p *playbackEngine) setVolume(value float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.volume = clampFloat(value, 0, 1)
	p.applyVolumeLocked()
	if p.current != nil {
		logEvent("playback_volume_changed", "track_id", p.current.track.ID, "value", p.volume, "muted", p.muted)
	}
}

func (p *playbackEngine) toggleMute() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.muted = !p.muted
	p.applyVolumeLocked()
	if p.current != nil {
		logEvent("playback_mute_toggled", "track_id", p.current.track.ID, "muted", p.muted, "volume", p.volume)
	}
}

func (p *playbackEngine) setShuffle(enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.shuffleEnabled = enabled
	if enabled && len(p.queue) > 1 {
		currentID := ""
		if p.current != nil {
			currentID = p.current.track.ID
		}
		p.queue = shuffleQueue(p.queue, currentID)
		p.syncCurrentQueueLocked()
	}
	logEvent("playback_shuffle_changed", "enabled", enabled, "queue_length", len(p.queue))
}

func (p *playbackEngine) cycleRepeat() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch p.repeatMode {
	case "off":
		p.repeatMode = "all"
	case "all":
		p.repeatMode = "one"
	default:
		p.repeatMode = "off"
	}
	logEvent("playback_repeat_changed", "repeat_mode", p.repeatMode)
	return p.repeatMode
}

func (p *playbackEngine) addToQueue(trackID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.lookup[trackID]; !ok {
		logEvent("playback_queue_add_failed", "track_id", trackID, "reason", "track_not_found")
		return
	}
	if len(p.queue) == 0 && p.current == nil {
		p.queue = []string{trackID}
		p.queueSource = "manual"
		logEvent("playback_queue_add", "track_id", trackID, "queue_source", p.queueSource, "queue_length", len(p.queue))
		return
	}
	for _, existing := range p.queue {
		if existing == trackID {
			return
		}
	}
	p.queue = append(p.queue, trackID)
	p.syncCurrentQueueLocked()
	logEvent("playback_queue_add", "track_id", trackID, "queue_source", p.queueSource, "queue_length", len(p.queue))
}

func (p *playbackEngine) removeFromQueue(trackID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.queue) == 0 {
		return
	}
	index := -1
	for i, existing := range p.queue {
		if existing == trackID {
			index = i
			break
		}
	}
	if index < 0 {
		return
	}
	p.queue = append(p.queue[:index], p.queue[index+1:]...)
	logEvent("playback_queue_remove", "track_id", trackID, "queue_length", len(p.queue))
	if p.current != nil && p.current.track.ID == trackID {
		p.stopLocked()
		if len(p.queue) > 0 {
			nextID := p.queue[0]
			if track, ok := p.lookup[nextID]; ok {
				_ = p.startLocked(track, p.queue, p.queueSource, true)
			}
		} else {
			p.current = nil
		}
		return
	}
	p.syncCurrentQueueLocked()
}

func (p *playbackEngine) clearQueue() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopLocked()
	p.queue = []string{}
	p.queueSource = ""
	p.current = nil
	p.lastError = ""
	logEvent("playback_queue_cleared")
}

func (p *playbackEngine) shuffleQueue() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.queue) < 2 {
		return
	}
	currentID := ""
	if p.current != nil {
		currentID = p.current.track.ID
	}
	p.queue = shuffleQueue(p.queue, currentID)
	p.syncCurrentQueueLocked()
	logEvent("playback_queue_shuffled", "queue_length", len(p.queue))
}

func (p *playbackEngine) moveQueue(trackID string, delta int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if delta == 0 || len(p.queue) < 2 {
		return false
	}
	currentID := ""
	if p.current != nil {
		currentID = p.current.track.ID
		if currentID == trackID {
			return false
		}
	}
	index := -1
	for i, existing := range p.queue {
		if existing == trackID {
			index = i
			break
		}
	}
	if index < 0 {
		return false
	}
	minIndex := 0
	if currentID != "" {
		currentIndex := -1
		for i, existing := range p.queue {
			if existing == currentID {
				currentIndex = i
				break
			}
		}
		if currentIndex >= 0 {
			minIndex = currentIndex + 1
		}
	}
	target := index + delta
	if target < minIndex {
		target = minIndex
	}
	if target >= len(p.queue) {
		target = len(p.queue) - 1
	}
	if target == index {
		return false
	}
	item := p.queue[index]
	if target > index {
		copy(p.queue[index:target], p.queue[index+1:target+1])
	} else {
		copy(p.queue[target+1:index+1], p.queue[target:index])
	}
	p.queue[target] = item
	p.syncCurrentQueueLocked()
	logEvent("playback_queue_moved", "track_id", trackID, "from_index", index, "to_index", target, "queue_length", len(p.queue))
	return true
}

func (p *playbackEngine) startLocked(track TrackRecord, queueIDs []string, queueSource string, stopCurrent bool) error {
	if track.AudioPath == "" {
		logEvent("playback_start_failed", "track_id", track.ID, "reason", "audio_path_missing")
		return errors.New("audio file missing")
	}
	if _, err := os.Stat(track.AudioPath); err != nil {
		logEvent("playback_start_failed", "track_id", track.ID, "audio_path", track.AudioPath, "error", err.Error())
		return errors.New("audio file missing")
	}
	if stopCurrent {
		p.stopLocked()
	}
	file, streamer, format, err := openAudioDecoder(track.AudioPath)
	if err != nil {
		logEvent("playback_start_failed", "track_id", track.ID, "audio_path", track.AudioPath, "error", err.Error())
		return err
	}
	if !p.speakerReady {
		if err := speaker.Init(format.SampleRate, format.SampleRate.N(time.Second/10)); err != nil {
			_ = streamer.Close()
			_ = file.Close()
			logEvent("playback_start_failed", "track_id", track.ID, "audio_path", track.AudioPath, "reason", "speaker_init_failed", "error", err.Error())
			return fmt.Errorf("initialize audio speaker: %w", err)
		}
		p.speakerRate = format.SampleRate
		p.speakerReady = true
	} else if format.SampleRate != p.speakerRate {
		// beep's speaker is bound to one sample rate. Recreate the device at the
		// new track's rate instead of rejecting an otherwise valid file. The
		// current streamer has not been added to the mixer yet, so it is safe to
		// close it if device initialization fails.
		previousRate := p.speakerRate
		speaker.Clear()
		speaker.Close()
		if err := speaker.Init(format.SampleRate, format.SampleRate.N(time.Second/10)); err != nil {
			p.speakerReady = false
			_ = streamer.Close()
			_ = file.Close()
			logEvent("playback_start_failed", "track_id", track.ID, "audio_path", track.AudioPath, "reason", "speaker_reinit_failed", "current_rate", previousRate, "new_rate", format.SampleRate, "error", err.Error())
			return fmt.Errorf("reinitialize audio speaker for sample rate %d: %w", format.SampleRate, err)
		}
		p.speakerRate = format.SampleRate
		logEvent("playback_speaker_reinitialized", "track_id", track.ID, "previous_rate", previousRate, "new_rate", format.SampleRate)
	}
	queueIDs = normalizeQueue(queueIDs, track.ID)
	volumeControl := &effects.Volume{
		Streamer: streamer,
		Base:     2,
		Volume:   volumeGain(p.volume),
		Silent:   p.muted,
	}
	ctrl := &beep.Ctrl{Streamer: volumeControl, Paused: false}
	playback := &activePlayback{
		track:         track,
		queue:         append([]string{}, queueIDs...),
		queueSource:   queueSource,
		file:          file,
		streamer:      streamer,
		ctrl:          ctrl,
		volumeControl: volumeControl,
		sampleRate:    format.SampleRate,
		samples:       streamer.Len(),
		startedAt:     time.Now().UTC(),
		pausedAt:      0,
		playing:       true,
		loading:       false,
	}
	p.current = playback
	p.lastError = ""
	speaker.Clear()
	speaker.Play(beep.Seq(ctrl, beep.Callback(func() {
		go p.onEnded(track.ID)
	})))
	logEvent("playback_started", "track_id", track.ID, "audio_path", track.AudioPath, "queue_source", queueSource, "queue_length", len(queueIDs))
	return nil
}

func openAudioDecoder(path string) (*os.File, beep.StreamSeekCloser, beep.Format, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, beep.Format{}, err
	}
	formatName := audioFormatFromPath(path)
	switch formatName {
	case audioFormatMP3:
		streamer, format, err := mp3.Decode(file)
		if err != nil {
			_ = file.Close()
			return nil, nil, beep.Format{}, err
		}
		return file, streamer, format, nil
	case audioFormatWAV:
		streamer, format, err := wav.Decode(file)
		if err != nil {
			_ = file.Close()
			return nil, nil, beep.Format{}, err
		}
		return file, streamer, format, nil
	case audioFormatFLAC:
		streamer, format, err := flac.Decode(file)
		if err != nil {
			_ = file.Close()
			return nil, nil, beep.Format{}, err
		}
		return file, streamer, format, nil
	case audioFormatOGG:
		streamer, format, err := vorbis.Decode(file)
		if err != nil {
			_ = file.Close()
			return nil, nil, beep.Format{}, err
		}
		return file, streamer, format, nil
	default:
		_ = file.Close()
		if formatName == "" {
			formatName = strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
		}
		if formatName == "" {
			formatName = "unknown"
		}
		return nil, nil, beep.Format{}, fmt.Errorf("unsupported audio format %s", formatName)
	}
}

func (p *playbackEngine) onEnded(trackID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current == nil || p.current.track.ID != trackID {
		return
	}
	logEvent("playback_ended", "track_id", trackID, "repeat_mode", p.repeatMode, "queue_length", len(p.queue))
	if p.repeatMode == "one" {
		_ = p.restartLocked()
		return
	}
	nextID := p.nextTrackIDLocked()
	if nextID == "" {
		p.stopLocked()
		return
	}
	track, ok := p.lookup[nextID]
	if !ok {
		p.stopLocked()
		return
	}
	_ = p.startLocked(track, p.queue, p.queueSource, true)
}

func (p *playbackEngine) restartLocked() error {
	if p.current == nil {
		return errors.New("no track selected")
	}
	return p.startLocked(p.current.track, p.queue, p.queueSource, true)
}

func (p *playbackEngine) syncCurrentQueueLocked() {
	if p.current == nil {
		return
	}
	p.current.queue = append([]string{}, p.queue...)
	p.current.queueSource = p.queueSource
}

func (p *playbackEngine) stopLocked() {
	p.stopSeekSilenceTimerLocked()
	if p.current == nil {
		return
	}
	if p.current.ctrl != nil {
		p.current.ctrl.Paused = true
	}
	speaker.Clear()
	if p.current.streamer != nil {
		_ = p.current.streamer.Close()
	}
	if p.current.file != nil {
		_ = p.current.file.Close()
	}
	p.current.playing = false
	p.current.loading = false
	p.current.pausedAt = 0
	logEvent("playback_stopped", "track_id", p.current.track.ID, "queue_source", p.queueSource)
}

func (p *playbackEngine) nextTrackIDLocked() string {
	if p.current == nil || len(p.queue) == 0 {
		return ""
	}
	currentID := p.current.track.ID
	for index, trackID := range p.queue {
		if trackID == currentID {
			return p.queue[(index+1)%len(p.queue)]
		}
	}
	return ""
}

func (p *playbackEngine) previousTrackIDLocked() string {
	if p.current == nil || len(p.queue) == 0 {
		return ""
	}
	currentID := p.current.track.ID
	for index, trackID := range p.queue {
		if trackID == currentID {
			if index == 0 {
				return p.queue[len(p.queue)-1]
			}
			return p.queue[index-1]
		}
	}
	return ""
}

func normalizeQueue(queueIDs []string, currentID string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(queueIDs))
	for _, id := range queueIDs {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	if currentID != "" {
		if _, ok := seen[currentID]; !ok {
			result = append([]string{currentID}, result...)
		}
	}
	return result
}

func shuffleQueue(queueIDs []string, currentID string) []string {
	if len(queueIDs) < 2 {
		return append([]string{}, queueIDs...)
	}
	head := currentID
	items := make([]string, 0, len(queueIDs))
	for _, id := range queueIDs {
		if id != head {
			items = append(items, id)
		}
	}
	rand.New(rand.NewSource(time.Now().UnixNano())).Shuffle(len(items), func(i, j int) {
		items[i], items[j] = items[j], items[i]
	})
	if head == "" && len(items) > 0 {
		head = items[0]
		items = items[1:]
	}
	return append([]string{head}, items...)
}

func volumeGain(value float64) float64 {
	if value <= 0 {
		return -math.MaxFloat64
	}
	return math.Log2(value)
}

func applyVolume(control *effects.Volume, value float64, muted bool) {
	if control == nil {
		return
	}
	control.Silent = muted || value <= 0
	control.Volume = volumeGain(value)
}

func (p *playbackEngine) silenceSeekLocked() {
	speaker.Lock()
	defer speaker.Unlock()
	p.silenceSeekLockedNoLock()
}

func (p *playbackEngine) silenceSeekLockedNoLock() {
	if p.current == nil || p.current.volumeControl == nil {
		return
	}
	applyVolume(p.current.volumeControl, p.volume, true)
}

func (p *playbackEngine) restoreVolumeLocked() {
	speaker.Lock()
	defer speaker.Unlock()
	p.restoreVolumeLockedNoLock()
}

func (p *playbackEngine) restoreVolumeLockedNoLock() {
	if p.current == nil || p.current.volumeControl == nil {
		return
	}
	applyVolume(p.current.volumeControl, p.volume, p.muted)
}

func (p *playbackEngine) stopSeekSilenceTimerLocked() {
	if p.seekSilenceTimer == nil {
		return
	}
	p.seekSilenceTimer.Stop()
	p.seekSilenceTimer = nil
}

func (p *playbackEngine) scheduleSeekVolumeRestoreLocked() {
	if p.current == nil || p.current.volumeControl == nil {
		return
	}
	p.stopSeekSilenceTimerLocked()
	p.seekSilenceTimer = time.AfterFunc(90*time.Millisecond, func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.current != nil && p.current.volumeControl != nil {
			applyVolume(p.current.volumeControl, p.volume, p.muted)
		}
		p.seekSilenceTimer = nil
	})
}

func (p *playbackEngine) applyVolumeLocked() {
	if p.current == nil || p.current.volumeControl == nil {
		return
	}
	applyVolume(p.current.volumeControl, p.volume, p.muted)
}

func clampFloat(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func (p *activePlayback) durationSeconds() float64 {
	if p.sampleRate <= 0 {
		return 0
	}
	return float64(p.samples) / float64(p.sampleRate)
}

func (p *activePlayback) currentSeconds() float64 {
	if p.sampleRate <= 0 {
		return 0
	}
	played := float64(p.pausedAt)
	if p.playing {
		played = float64(p.pausedAt) + time.Since(p.startedAt).Seconds()*float64(p.sampleRate)
	}
	seconds := played / float64(p.sampleRate)
	if duration := p.durationSeconds(); duration > 0 && seconds > duration {
		return duration
	}
	return seconds
}

func (p *activePlayback) pauseLocked() {
	if p.ctrl != nil {
		p.ctrl.Paused = true
	}
	p.pausedAt = int(p.currentSeconds() * float64(p.sampleRate))
	p.playing = false
}

func (p *activePlayback) resumeLocked() {
	if p.ctrl != nil {
		p.ctrl.Paused = false
	}
	p.startedAt = time.Now().UTC()
	p.playing = true
}
