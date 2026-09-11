package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

// SmartPlaylistSchemaVersion is the version of the persisted smart-playlist
// definition. A definition is deliberately independent of the catalog and
// can therefore be inspected, edited, and migrated without loading the app.
const SmartPlaylistSchemaVersion = 1

// SmartPlaylistField identifies a catalog or local-history value that can be
// queried by a smart playlist rule.
type SmartPlaylistField string

const (
	SmartPlaylistFieldArtist               SmartPlaylistField = "artist"
	SmartPlaylistFieldAlbum                SmartPlaylistField = "album"
	SmartPlaylistFieldTitle                SmartPlaylistField = "title"
	SmartPlaylistFieldGenre                SmartPlaylistField = "genre"
	SmartPlaylistFieldMood                 SmartPlaylistField = "mood"
	SmartPlaylistFieldYear                 SmartPlaylistField = "year"
	SmartPlaylistFieldDecade               SmartPlaylistField = "decade"
	SmartPlaylistFieldCreatedAt            SmartPlaylistField = "createdAt"
	SmartPlaylistFieldRecentlyAdded        SmartPlaylistField = "recentlyAdded"
	SmartPlaylistFieldLastPlayedAt         SmartPlaylistField = "lastPlayedAt"
	SmartPlaylistFieldRecentlyPlayed       SmartPlaylistField = "recentlyPlayed"
	SmartPlaylistFieldPlayCount            SmartPlaylistField = "playCount"
	SmartPlaylistFieldNeverPlayed          SmartPlaylistField = "neverPlayed"
	SmartPlaylistFieldConfidence           SmartPlaylistField = "confidence"
	SmartPlaylistFieldMetadataConfidence   SmartPlaylistField = "metadataConfidence"
	SmartPlaylistFieldEnrichmentConfidence SmartPlaylistField = "enrichmentConfidence"
	SmartPlaylistFieldMissingMetadata      SmartPlaylistField = "missingMetadata"
	SmartPlaylistFieldDuplicateCandidate   SmartPlaylistField = "duplicateCandidate"
	SmartPlaylistFieldVersion              SmartPlaylistField = "version"
	SmartPlaylistFieldIncompleteAlbum      SmartPlaylistField = "incompleteAlbum"
	SmartPlaylistFieldHasLyrics            SmartPlaylistField = "hasLyrics"
	SmartPlaylistFieldHasTimedLyrics       SmartPlaylistField = "hasTimedLyrics"
	SmartPlaylistFieldHasArtwork           SmartPlaylistField = "hasArtwork"
	SmartPlaylistFieldHasVideo             SmartPlaylistField = "hasVideo"
	SmartPlaylistFieldDurationSeconds      SmartPlaylistField = "durationSeconds"
)

// SmartPlaylistOperator is the comparison performed by a leaf rule.
type SmartPlaylistOperator string

const (
	SmartPlaylistOperatorEquals         SmartPlaylistOperator = "equals"
	SmartPlaylistOperatorNotEquals      SmartPlaylistOperator = "notEquals"
	SmartPlaylistOperatorContains       SmartPlaylistOperator = "contains"
	SmartPlaylistOperatorIn             SmartPlaylistOperator = "in"
	SmartPlaylistOperatorMissing        SmartPlaylistOperator = "missing"
	SmartPlaylistOperatorNotMissing     SmartPlaylistOperator = "notMissing"
	SmartPlaylistOperatorGreaterThan    SmartPlaylistOperator = "greaterThan"
	SmartPlaylistOperatorGreaterOrEqual SmartPlaylistOperator = "greaterOrEqual"
	SmartPlaylistOperatorLessThan       SmartPlaylistOperator = "lessThan"
	SmartPlaylistOperatorLessOrEqual    SmartPlaylistOperator = "lessOrEqual"
	SmartPlaylistOperatorBetween        SmartPlaylistOperator = "between"
	SmartPlaylistOperatorBefore         SmartPlaylistOperator = "before"
	SmartPlaylistOperatorAfter          SmartPlaylistOperator = "after"
	SmartPlaylistOperatorIsTrue         SmartPlaylistOperator = "isTrue"
	SmartPlaylistOperatorIsFalse        SmartPlaylistOperator = "isFalse"
)

// SmartPlaylistPredicate is one inspectable leaf in a SmartPlaylistRule.
// Value is used by unary comparisons; Values is used by "in" and "between".
// Dates use RFC3339 strings so definitions remain portable and human-editable.
type SmartPlaylistPredicate struct {
	Field    SmartPlaylistField    `json:"field"`
	Operator SmartPlaylistOperator `json:"operator"`
	Value    string                `json:"value,omitempty"`
	Values   []string              `json:"values,omitempty"`
}

// SmartPlaylistRule is a boolean expression. Exactly one of All, Any, Not,
// or Predicate must be present at each node. This explicit recursive shape
// keeps JSON round trips deterministic and avoids interface-based decoding.
type SmartPlaylistRule struct {
	All       []SmartPlaylistRule     `json:"all,omitempty"`
	Any       []SmartPlaylistRule     `json:"any,omitempty"`
	Not       *SmartPlaylistRule      `json:"not,omitempty"`
	Predicate *SmartPlaylistPredicate `json:"predicate,omitempty"`
}

// SmartPlaylistSortField is the primary field used to order generated tracks.
type SmartPlaylistSortField string

const (
	SmartPlaylistSortTitle           SmartPlaylistSortField = "title"
	SmartPlaylistSortArtist          SmartPlaylistSortField = "artist"
	SmartPlaylistSortAlbum           SmartPlaylistSortField = "album"
	SmartPlaylistSortGenre           SmartPlaylistSortField = "genre"
	SmartPlaylistSortYear            SmartPlaylistSortField = "year"
	SmartPlaylistSortCreatedAt       SmartPlaylistSortField = "createdAt"
	SmartPlaylistSortLastPlayedAt    SmartPlaylistSortField = "lastPlayedAt"
	SmartPlaylistSortPlayCount       SmartPlaylistSortField = "playCount"
	SmartPlaylistSortDurationSeconds SmartPlaylistSortField = "durationSeconds"
	SmartPlaylistSortID              SmartPlaylistSortField = "id"
)

// SmartPlaylistSort controls ordering. Sorting is stable: tracks with equal
// sort values retain their input/catalog order, including in descending mode.
type SmartPlaylistSort struct {
	Field      SmartPlaylistSortField `json:"field"`
	Descending bool                   `json:"descending,omitempty"`
}

// SmartPlaylistTrackContext contains optional local facts that are not part of
// TrackRecord. It is intentionally caller-supplied so listening history stays
// local and opt-in; an absent context simply behaves like an unplayed track
// with no derived facts.
type SmartPlaylistTrackContext struct {
	LastPlayedAt            *time.Time `json:"lastPlayedAt,omitempty"`
	PlayCount               int        `json:"playCount,omitempty"`
	Moods                   []string   `json:"moods,omitempty"`
	DuplicateCandidate      bool       `json:"duplicateCandidate,omitempty"`
	VersionKey              string     `json:"versionKey,omitempty"`
	AlbumTrackCount         int        `json:"albumTrackCount,omitempty"`
	ExpectedAlbumTrackCount int        `json:"expectedAlbumTrackCount,omitempty"`
}

// SmartPlaylistDefinition is the versioned, serializable definition of a
// generated playlist. ID is the stable identity used by the resulting
// Playlist; it must be supplied by the caller rather than generated from
// mutable track data.
type SmartPlaylistDefinition struct {
	SchemaVersion int               `json:"schemaVersion"`
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Description   string            `json:"description,omitempty"`
	Rule          SmartPlaylistRule `json:"rule"`
	Sort          SmartPlaylistSort `json:"sort,omitempty"`
	Limit         int               `json:"limit,omitempty"`
}

// SmartPlaylistMatch pairs a matching track with the explanation that can be
// shown in a preview or saved alongside a future UI result.
type SmartPlaylistMatch struct {
	Track       TrackRecord               `json:"track"`
	Context     SmartPlaylistTrackContext `json:"context,omitempty"`
	Explanation SmartPlaylistExplanation  `json:"explanation"`
	inputIndex  int
}

// SmartPlaylistEvaluation is the offline result of evaluating a definition.
// TotalMatches is reported before Limit is applied, allowing a preview to
// display both the full count and the generated Playlist contents.
type SmartPlaylistEvaluation struct {
	Playlist     Playlist             `json:"playlist"`
	Matches      []SmartPlaylistMatch `json:"matches"`
	TotalMatches int                  `json:"totalMatches"`
}

// SmartPlaylistExplanation is a tree mirroring the rule. Leaf nodes contain
// the normalized actual value, expected value, and a short human-readable
// reason; group nodes retain their child explanations for explainability.
type SmartPlaylistExplanation struct {
	Matched  bool                       `json:"matched"`
	Kind     string                     `json:"kind"`
	Field    SmartPlaylistField         `json:"field,omitempty"`
	Operator SmartPlaylistOperator      `json:"operator,omitempty"`
	Expected string                     `json:"expected,omitempty"`
	Actual   string                     `json:"actual,omitempty"`
	Reason   string                     `json:"reason"`
	Children []SmartPlaylistExplanation `json:"children,omitempty"`
}

// DefaultSmartPlaylistSort returns the predictable default used when a
// definition omits a sort field.
func DefaultSmartPlaylistSort() SmartPlaylistSort {
	return SmartPlaylistSort{Field: SmartPlaylistSortTitle}
}

// Validate checks a smart-playlist definition before it is persisted or
// evaluated. Limits protect callers from accidentally constructing enormous
// recursive expressions while remaining generous for hand-authored rules.
func (d SmartPlaylistDefinition) Validate() error {
	if d.SchemaVersion != 0 && d.SchemaVersion != SmartPlaylistSchemaVersion {
		return fmt.Errorf("unsupported smart playlist schema version %d", d.SchemaVersion)
	}
	if strings.TrimSpace(d.ID) == "" {
		return errors.New("smart playlist id is required")
	}
	if strings.TrimSpace(d.Name) == "" {
		return errors.New("smart playlist name is required")
	}
	if d.Limit < 0 {
		return errors.New("smart playlist limit cannot be negative")
	}
	if d.Limit > 100000 {
		return errors.New("smart playlist limit exceeds 100000 tracks")
	}
	if err := d.Sort.Validate(); err != nil {
		return err
	}
	state := smartPlaylistRuleValidationState{}
	if err := validateSmartPlaylistRule(d.Rule, "$", 0, &state); err != nil {
		return err
	}
	return nil
}

// ValidateSmartPlaylistRule validates a standalone rule for callers that do
// not yet have a full named definition.
func ValidateSmartPlaylistRule(rule SmartPlaylistRule) error {
	state := smartPlaylistRuleValidationState{}
	return validateSmartPlaylistRule(rule, "$", 0, &state)
}

// ValidateSmartPlaylistDefinition is the function form of
// SmartPlaylistDefinition.Validate for serialization and service boundaries.
func ValidateSmartPlaylistDefinition(definition SmartPlaylistDefinition) error {
	return definition.Validate()
}

type smartPlaylistRuleValidationState struct {
	nodes int
}

func validateSmartPlaylistRule(rule SmartPlaylistRule, path string, depth int, state *smartPlaylistRuleValidationState) error {
	if depth > 32 {
		return fmt.Errorf("%s exceeds smart playlist rule depth 32", path)
	}
	state.nodes++
	if state.nodes > 1024 {
		return errors.New("smart playlist rule exceeds 1024 nodes")
	}
	branches := 0
	if len(rule.All) > 0 {
		branches++
	}
	if len(rule.Any) > 0 {
		branches++
	}
	if rule.Not != nil {
		branches++
	}
	if rule.Predicate != nil {
		branches++
	}
	if branches != 1 {
		return fmt.Errorf("%s must contain exactly one non-empty rule branch", path)
	}
	if len(rule.All) > 0 {
		for i, child := range rule.All {
			if err := validateSmartPlaylistRule(child, fmt.Sprintf("%s.all[%d]", path, i), depth+1, state); err != nil {
				return err
			}
		}
		return nil
	}
	if len(rule.Any) > 0 {
		for i, child := range rule.Any {
			if err := validateSmartPlaylistRule(child, fmt.Sprintf("%s.any[%d]", path, i), depth+1, state); err != nil {
				return err
			}
		}
		return nil
	}
	if rule.Not != nil {
		return validateSmartPlaylistRule(*rule.Not, path+".not", depth+1, state)
	}
	return validateSmartPlaylistPredicate(*rule.Predicate, path+".predicate")
}

func validateSmartPlaylistPredicate(predicate SmartPlaylistPredicate, path string) error {
	field := SmartPlaylistField(strings.TrimSpace(string(predicate.Field)))
	op := SmartPlaylistOperator(strings.TrimSpace(string(predicate.Operator)))
	if !isSmartPlaylistField(field) {
		return fmt.Errorf("%s has unsupported field %q", path, predicate.Field)
	}
	if !isSmartPlaylistOperator(op) {
		return fmt.Errorf("%s has unsupported operator %q", path, predicate.Operator)
	}
	if !smartPlaylistOperatorAllowed(field, op) {
		return fmt.Errorf("operator %q is not valid for field %q", op, field)
	}
	value := strings.TrimSpace(predicate.Value)
	values := trimSmartPlaylistValues(predicate.Values)
	switch op {
	case SmartPlaylistOperatorMissing, SmartPlaylistOperatorNotMissing, SmartPlaylistOperatorIsTrue, SmartPlaylistOperatorIsFalse:
		if value != "" || len(values) != 0 {
			return fmt.Errorf("%s operator %q does not accept a value", path, op)
		}
	case SmartPlaylistOperatorIn:
		if len(values) == 0 || value != "" {
			return fmt.Errorf("%s operator in requires values and no value", path)
		}
	case SmartPlaylistOperatorBetween:
		if len(values) != 2 || value != "" {
			return fmt.Errorf("%s operator between requires exactly two values", path)
		}
	default:
		if value == "" || len(values) != 0 {
			return fmt.Errorf("%s operator %q requires one value and no values array", path, op)
		}
	}
	if err := validateSmartPlaylistPredicateValues(field, op, value, values); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func validateSmartPlaylistPredicateValues(field SmartPlaylistField, op SmartPlaylistOperator, value string, values []string) error {
	if op == SmartPlaylistOperatorMissing || op == SmartPlaylistOperatorNotMissing {
		return nil
	}
	if field == SmartPlaylistFieldConfidence || field == SmartPlaylistFieldMetadataConfidence || field == SmartPlaylistFieldEnrichmentConfidence {
		check := append([]string{value}, values...)
		for _, candidate := range check {
			if confidenceRank(candidate) < 0 {
				return fmt.Errorf("unsupported confidence %q", candidate)
			}
		}
	}
	if smartPlaylistNumericField(field) {
		check := append([]string{value}, values...)
		for _, candidate := range check {
			parsed, err := strconv.Atoi(candidate)
			if err != nil {
				return fmt.Errorf("%q is not an integer", candidate)
			}
			if field == SmartPlaylistFieldDecade && parsed%10 != 0 {
				return fmt.Errorf("%q is not the first year of a decade", candidate)
			}
		}
	}
	if smartPlaylistDateField(field) {
		check := append([]string{value}, values...)
		for _, candidate := range check {
			if _, err := time.Parse(time.RFC3339, candidate); err != nil {
				return fmt.Errorf("%q is not an RFC3339 timestamp", candidate)
			}
		}
	}
	if smartPlaylistBooleanField(field) && (op == SmartPlaylistOperatorEquals || op == SmartPlaylistOperatorNotEquals) {
		if _, err := strconv.ParseBool(value); err != nil {
			return fmt.Errorf("%q is not a boolean", value)
		}
	}
	return nil
}

// Validate checks whether this sort can be used by a smart playlist.
func (s SmartPlaylistSort) Validate() error {
	if s.Field == "" {
		return nil
	}
	if !isSmartPlaylistSortField(s.Field) {
		return fmt.Errorf("unsupported smart playlist sort field %q", s.Field)
	}
	return nil
}

// MarshalSmartPlaylist serializes a validated definition with an explicit
// schema version. A zero version is filled with the current version for easy
// construction in Go; unknown non-zero versions are rejected.
func MarshalSmartPlaylist(definition SmartPlaylistDefinition) ([]byte, error) {
	if definition.SchemaVersion == 0 {
		definition.SchemaVersion = SmartPlaylistSchemaVersion
	}
	if err := definition.Validate(); err != nil {
		return nil, err
	}
	return json.MarshalIndent(definition, "", "  ")
}

// UnmarshalSmartPlaylist parses one current-version definition. Unknown JSON
// fields are rejected so hand edits cannot silently create rules the matcher
// does not understand.
func UnmarshalSmartPlaylist(data []byte) (SmartPlaylistDefinition, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var definition SmartPlaylistDefinition
	if err := decoder.Decode(&definition); err != nil {
		return SmartPlaylistDefinition{}, fmt.Errorf("decode smart playlist: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return SmartPlaylistDefinition{}, errors.New("smart playlist contains multiple JSON values")
	} else if !errors.Is(err, io.EOF) {
		return SmartPlaylistDefinition{}, fmt.Errorf("decode trailing smart playlist data: %w", err)
	}
	if definition.SchemaVersion != SmartPlaylistSchemaVersion {
		return SmartPlaylistDefinition{}, fmt.Errorf("unsupported smart playlist schema version %d", definition.SchemaVersion)
	}
	if err := definition.Validate(); err != nil {
		return SmartPlaylistDefinition{}, err
	}
	return definition, nil
}

// Match evaluates a rule without requiring persistence or network access.
func (r SmartPlaylistRule) Match(track TrackRecord, context SmartPlaylistTrackContext) (bool, error) {
	explanation, err := r.Explain(track, context)
	if err != nil {
		return false, err
	}
	return explanation.Matched, nil
}

// Explain evaluates a rule and returns a tree explaining every branch and
// leaf. The rule is validated before matching so callers receive actionable
// errors rather than a false result for malformed definitions.
func (r SmartPlaylistRule) Explain(track TrackRecord, context SmartPlaylistTrackContext) (SmartPlaylistExplanation, error) {
	if err := ValidateSmartPlaylistRule(r); err != nil {
		return SmartPlaylistExplanation{}, err
	}
	return explainSmartPlaylistRule(r, track, context), nil
}

func explainSmartPlaylistRule(rule SmartPlaylistRule, track TrackRecord, context SmartPlaylistTrackContext) SmartPlaylistExplanation {
	if len(rule.All) > 0 {
		children := make([]SmartPlaylistExplanation, 0, len(rule.All))
		matched := true
		for _, child := range rule.All {
			explanation := explainSmartPlaylistRule(child, track, context)
			children = append(children, explanation)
			matched = matched && explanation.Matched
		}
		return SmartPlaylistExplanation{Matched: matched, Kind: "all", Reason: smartPlaylistGroupReason("all", matched), Children: children}
	}
	if len(rule.Any) > 0 {
		children := make([]SmartPlaylistExplanation, 0, len(rule.Any))
		matched := false
		for _, child := range rule.Any {
			explanation := explainSmartPlaylistRule(child, track, context)
			children = append(children, explanation)
			matched = matched || explanation.Matched
		}
		return SmartPlaylistExplanation{Matched: matched, Kind: "any", Reason: smartPlaylistGroupReason("any", matched), Children: children}
	}
	if rule.Not != nil {
		child := explainSmartPlaylistRule(*rule.Not, track, context)
		return SmartPlaylistExplanation{Matched: !child.Matched, Kind: "not", Reason: smartPlaylistGroupReason("not", !child.Matched), Children: []SmartPlaylistExplanation{child}}
	}
	predicate := *rule.Predicate
	actual := smartPlaylistActualValue(track, context, predicate.Field)
	matched := matchSmartPlaylistPredicate(predicate, actual)
	expected := smartPlaylistPredicateExpectation(predicate)
	actualText := actual.display()
	return SmartPlaylistExplanation{
		Matched: matched, Kind: "predicate", Field: predicate.Field, Operator: predicate.Operator,
		Expected: expected, Actual: actualText,
		Reason: fmt.Sprintf("%s %s %s (actual %s)", predicate.Field, predicate.Operator, expected, actualText),
	}
}

func smartPlaylistGroupReason(kind string, matched bool) string {
	if matched {
		return fmt.Sprintf("all %s rules matched", kind)
	}
	return fmt.Sprintf("%s rule did not match", kind)
}

func smartPlaylistPredicateExpectation(predicate SmartPlaylistPredicate) string {
	if len(predicate.Values) > 0 {
		return "[" + strings.Join(trimSmartPlaylistValues(predicate.Values), ", ") + "]"
	}
	if predicate.Value == "" {
		return "(none)"
	}
	return predicate.Value
}

// EvaluateSmartPlaylist filters, explains, stably sorts, and limits tracks,
// returning an existing Playlist value that can be passed to the normal queue.
// The history map is optional and is keyed by TrackRecord.ID.
func EvaluateSmartPlaylist(definition SmartPlaylistDefinition, tracks []TrackRecord, history map[string]SmartPlaylistTrackContext) (SmartPlaylistEvaluation, error) {
	if definition.SchemaVersion == 0 {
		definition.SchemaVersion = SmartPlaylistSchemaVersion
	}
	if err := definition.Validate(); err != nil {
		return SmartPlaylistEvaluation{}, err
	}
	matches := make([]SmartPlaylistMatch, 0, len(tracks))
	for index, track := range tracks {
		context := SmartPlaylistTrackContext{}
		if history != nil {
			context = history[track.ID]
		}
		explanation, err := definition.Rule.Explain(track, context)
		if err != nil {
			return SmartPlaylistEvaluation{}, err
		}
		if explanation.Matched {
			matches = append(matches, SmartPlaylistMatch{Track: track, Context: context, Explanation: explanation, inputIndex: index})
		}
	}
	total := len(matches)
	sortSpec := definition.Sort
	if sortSpec.Field == "" {
		sortSpec = DefaultSmartPlaylistSort()
	}
	if err := SortSmartPlaylistMatches(matches, sortSpec); err != nil {
		return SmartPlaylistEvaluation{}, err
	}
	if definition.Limit > 0 && len(matches) > definition.Limit {
		matches = matches[:definition.Limit]
	}
	trackIDs := make([]string, 0, len(matches))
	for _, match := range matches {
		trackIDs = append(trackIDs, match.Track.ID)
	}
	return SmartPlaylistEvaluation{
		Playlist: Playlist{ID: definition.ID, Name: definition.Name, Description: definition.Description, TrackIDs: trackIDs},
		Matches:  matches, TotalMatches: total,
	}, nil
}

// SortSmartPlaylistMatches performs an in-place stable sort. Missing values
// are kept after present values in either direction, and equal values retain
// their original input order.
func SortSmartPlaylistMatches(matches []SmartPlaylistMatch, spec SmartPlaylistSort) error {
	if err := spec.Validate(); err != nil {
		return err
	}
	if spec.Field == "" {
		return nil
	}
	sort.SliceStable(matches, func(i, j int) bool {
		left := smartPlaylistSortValueForMatch(matches[i], spec.Field)
		right := smartPlaylistSortValueForMatch(matches[j], spec.Field)
		if left.missing != right.missing {
			return !left.missing
		}
		if left.missing {
			return false
		}
		comparison := compareSmartPlaylistSortValues(left, right)
		if spec.Descending {
			return comparison > 0
		}
		return comparison < 0
	})
	return nil
}

type smartPlaylistSortValue struct {
	missing bool
	text    string
	number  int
	date    time.Time
	isDate  bool
}

func compareSmartPlaylistSortValues(left, right smartPlaylistSortValue) int {
	if left.isDate || right.isDate {
		if left.date.Before(right.date) {
			return -1
		}
		if left.date.After(right.date) {
			return 1
		}
		return 0
	}
	if left.text != "" || right.text != "" {
		return strings.Compare(left.text, right.text)
	}
	if left.number < right.number {
		return -1
	}
	if left.number > right.number {
		return 1
	}
	return 0
}

func smartPlaylistSortValueForMatch(match SmartPlaylistMatch, field SmartPlaylistSortField) smartPlaylistSortValue {
	track := match.Track
	context := match.Context
	switch field {
	case SmartPlaylistSortTitle:
		return smartPlaylistTextSortValue(track.Title)
	case SmartPlaylistSortArtist:
		return smartPlaylistTextSortValue(track.Artist)
	case SmartPlaylistSortAlbum:
		return smartPlaylistTextSortValue(track.Album)
	case SmartPlaylistSortGenre:
		return smartPlaylistTextSortValue(track.Genre)
	case SmartPlaylistSortYear:
		year, ok := smartPlaylistYear(track.Year)
		return smartPlaylistNumberSortValue(year, !ok)
	case SmartPlaylistSortCreatedAt:
		return smartPlaylistDateSortValue(track.CreatedAt)
	case SmartPlaylistSortLastPlayedAt:
		if context.LastPlayedAt == nil {
			return smartPlaylistSortValue{missing: true}
		}
		return smartPlaylistDateSortValue(*context.LastPlayedAt)
	case SmartPlaylistSortPlayCount:
		return smartPlaylistNumberSortValue(context.PlayCount, false)
	case SmartPlaylistSortDurationSeconds:
		if track.DurationSeconds == nil {
			return smartPlaylistSortValue{missing: true}
		}
		return smartPlaylistNumberSortValue(*track.DurationSeconds, false)
	case SmartPlaylistSortID:
		return smartPlaylistTextSortValue(track.ID)
	default:
		return smartPlaylistSortValue{missing: true}
	}
}

func smartPlaylistTextSortValue(value string) smartPlaylistSortValue {
	value = strings.ToLower(strings.TrimSpace(value))
	return smartPlaylistSortValue{missing: value == "", text: value}
}

func smartPlaylistNumberSortValue(value int, missing bool) smartPlaylistSortValue {
	return smartPlaylistSortValue{missing: missing, number: value}
}

func smartPlaylistDateSortValue(value time.Time) smartPlaylistSortValue {
	return smartPlaylistSortValue{missing: value.IsZero(), date: value, isDate: true}
}

func matchSmartPlaylistPredicate(predicate SmartPlaylistPredicate, actual smartPlaylistActual) bool {
	op := predicate.Operator
	if op == SmartPlaylistOperatorMissing {
		return actual.missing
	}
	if op == SmartPlaylistOperatorNotMissing {
		return !actual.missing
	}
	if actual.missing {
		return false
	}
	if op == SmartPlaylistOperatorIsTrue {
		return actual.booleanValue
	}
	if op == SmartPlaylistOperatorIsFalse {
		return !actual.booleanValue
	}
	if predicate.Field == SmartPlaylistFieldConfidence || predicate.Field == SmartPlaylistFieldMetadataConfidence || predicate.Field == SmartPlaylistFieldEnrichmentConfidence {
		actualRank := confidenceRank(actual.textValue)
		expectedRank := confidenceRank(predicate.Value)
		if actualRank < 0 || expectedRank < 0 {
			return false
		}
		switch op {
		case SmartPlaylistOperatorGreaterOrEqual:
			return actualRank >= expectedRank
		case SmartPlaylistOperatorLessOrEqual:
			return actualRank <= expectedRank
		}
	}
	if actual.isBoolean {
		expected, err := strconv.ParseBool(strings.TrimSpace(predicate.Value))
		if err != nil {
			return false
		}
		switch op {
		case SmartPlaylistOperatorEquals:
			return actual.booleanValue == expected
		case SmartPlaylistOperatorNotEquals:
			return actual.booleanValue != expected
		}
	}
	if actual.isDate {
		return matchSmartPlaylistDate(predicate, actual.dateValue)
	}
	if actual.isNumber {
		return matchSmartPlaylistNumber(predicate, actual.numberValue)
	}
	return matchSmartPlaylistText(predicate, actual.textValue, actual.textValues)
}

func matchSmartPlaylistText(predicate SmartPlaylistPredicate, actual string, actualValues []string) bool {
	actual = strings.ToLower(strings.TrimSpace(actual))
	value := strings.ToLower(strings.TrimSpace(predicate.Value))
	containsValue := func(candidate string) bool { return strings.EqualFold(strings.TrimSpace(candidate), value) }
	if len(actualValues) > 0 {
		switch predicate.Operator {
		case SmartPlaylistOperatorEquals:
			for _, candidate := range actualValues {
				if containsValue(candidate) {
					return true
				}
			}
			return false
		case SmartPlaylistOperatorNotEquals:
			for _, candidate := range actualValues {
				if containsValue(candidate) {
					return false
				}
			}
			return true
		case SmartPlaylistOperatorIn:
			for _, candidate := range actualValues {
				for _, expected := range predicate.Values {
					if strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(expected)) {
						return true
					}
				}
			}
			return false
		}
	}
	switch predicate.Operator {
	case SmartPlaylistOperatorEquals:
		return actual == value
	case SmartPlaylistOperatorNotEquals:
		return actual != value
	case SmartPlaylistOperatorContains:
		if len(actualValues) > 0 {
			for _, candidate := range actualValues {
				if containsValue(candidate) {
					return true
				}
			}
			return false
		}
		return strings.Contains(actual, value)
	case SmartPlaylistOperatorIn:
		for _, candidate := range predicate.Values {
			if actual == strings.ToLower(strings.TrimSpace(candidate)) {
				return true
			}
		}
	}
	return false
}

func matchSmartPlaylistNumber(predicate SmartPlaylistPredicate, actual int) bool {
	parse := func(value string) (int, bool) {
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		return parsed, err == nil
	}
	expected, ok := parse(predicate.Value)
	switch predicate.Operator {
	case SmartPlaylistOperatorEquals:
		return ok && actual == expected
	case SmartPlaylistOperatorNotEquals:
		return ok && actual != expected
	case SmartPlaylistOperatorGreaterThan:
		return ok && actual > expected
	case SmartPlaylistOperatorGreaterOrEqual:
		return ok && actual >= expected
	case SmartPlaylistOperatorLessThan:
		return ok && actual < expected
	case SmartPlaylistOperatorLessOrEqual:
		return ok && actual <= expected
	case SmartPlaylistOperatorBetween:
		if len(predicate.Values) != 2 {
			return false
		}
		low, lowOK := parse(predicate.Values[0])
		high, highOK := parse(predicate.Values[1])
		return lowOK && highOK && actual >= low && actual <= high
	}
	return false
}

func matchSmartPlaylistDate(predicate SmartPlaylistPredicate, actual time.Time) bool {
	parse := func(value string) (time.Time, bool) {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
		return parsed, err == nil
	}
	expected, ok := parse(predicate.Value)
	switch predicate.Operator {
	case SmartPlaylistOperatorEquals:
		return ok && actual.Equal(expected)
	case SmartPlaylistOperatorNotEquals:
		return ok && !actual.Equal(expected)
	case SmartPlaylistOperatorBefore:
		return ok && actual.Before(expected)
	case SmartPlaylistOperatorAfter:
		return ok && actual.After(expected)
	case SmartPlaylistOperatorBetween:
		if len(predicate.Values) != 2 {
			return false
		}
		low, lowOK := parse(predicate.Values[0])
		high, highOK := parse(predicate.Values[1])
		return lowOK && highOK && !actual.Before(low) && !actual.After(high)
	}
	return false
}

type smartPlaylistActual struct {
	missing      bool
	textValue    string
	textValues   []string
	numberValue  int
	dateValue    time.Time
	booleanValue bool
	isNumber     bool
	isDate       bool
	isBoolean    bool
}

func (a smartPlaylistActual) display() string {
	if a.missing {
		return "missing"
	}
	if a.isDate {
		return a.dateValue.UTC().Format(time.RFC3339)
	}
	if a.isNumber {
		return strconv.Itoa(a.numberValue)
	}
	if a.isBoolean {
		return strconv.FormatBool(a.booleanValue)
	}
	if len(a.textValues) > 0 {
		return "[" + strings.Join(a.textValues, ", ") + "]"
	}
	if a.textValue == "" {
		return "empty"
	}
	return a.textValue
}

func smartPlaylistActualValue(track TrackRecord, context SmartPlaylistTrackContext, field SmartPlaylistField) smartPlaylistActual {
	switch field {
	case SmartPlaylistFieldArtist:
		return smartPlaylistTextActual(track.Artist)
	case SmartPlaylistFieldAlbum:
		return smartPlaylistTextActual(track.Album)
	case SmartPlaylistFieldTitle:
		return smartPlaylistTextActual(track.Title)
	case SmartPlaylistFieldGenre:
		return smartPlaylistTextActual(track.Genre)
	case SmartPlaylistFieldMood:
		return smartPlaylistListActual(context.Moods)
	case SmartPlaylistFieldYear:
		value, ok := smartPlaylistYear(track.Year)
		return smartPlaylistActual{missing: !ok, numberValue: value, isNumber: ok}
	case SmartPlaylistFieldDecade:
		year, ok := smartPlaylistYear(track.Year)
		if ok {
			year = (year / 10) * 10
		}
		return smartPlaylistActual{missing: !ok, numberValue: year, isNumber: ok}
	case SmartPlaylistFieldCreatedAt, SmartPlaylistFieldRecentlyAdded:
		return smartPlaylistDateActual(track.CreatedAt)
	case SmartPlaylistFieldLastPlayedAt, SmartPlaylistFieldRecentlyPlayed:
		if context.LastPlayedAt == nil || context.LastPlayedAt.IsZero() {
			return smartPlaylistActual{missing: true, isDate: true}
		}
		return smartPlaylistDateActual(*context.LastPlayedAt)
	case SmartPlaylistFieldPlayCount:
		return smartPlaylistActual{numberValue: context.PlayCount, isNumber: true}
	case SmartPlaylistFieldNeverPlayed:
		return smartPlaylistBooleanActual(context.PlayCount <= 0 && (context.LastPlayedAt == nil || context.LastPlayedAt.IsZero()))
	case SmartPlaylistFieldConfidence:
		return smartPlaylistTextActual(track.Confidence)
	case SmartPlaylistFieldMetadataConfidence:
		return smartPlaylistTextActual(track.MetadataConfidence)
	case SmartPlaylistFieldEnrichmentConfidence:
		return smartPlaylistTextActual(track.EnrichmentConfidence)
	case SmartPlaylistFieldMissingMetadata:
		return smartPlaylistBooleanActual(SmartPlaylistMetadataMissing(track))
	case SmartPlaylistFieldDuplicateCandidate:
		return smartPlaylistBooleanActual(context.DuplicateCandidate)
	case SmartPlaylistFieldVersion:
		return smartPlaylistTextActual(context.VersionKey)
	case SmartPlaylistFieldIncompleteAlbum:
		if context.ExpectedAlbumTrackCount <= 0 {
			return smartPlaylistActual{missing: true, isBoolean: true}
		}
		return smartPlaylistBooleanActual(context.AlbumTrackCount < context.ExpectedAlbumTrackCount)
	case SmartPlaylistFieldHasLyrics:
		return smartPlaylistBooleanActual(track.LyricsIncluded || strings.TrimSpace(track.LyricsPath) != "" || strings.TrimSpace(track.LRCPath) != "")
	case SmartPlaylistFieldHasTimedLyrics:
		return smartPlaylistBooleanActual(track.HasTimedLyrics)
	case SmartPlaylistFieldHasArtwork:
		return smartPlaylistBooleanActual(strings.TrimSpace(track.ArtworkPath) != "" || strings.TrimSpace(track.ArtworkURL) != "" || strings.TrimSpace(track.ArtworkMediaURL) != "")
	case SmartPlaylistFieldHasVideo:
		return smartPlaylistBooleanActual(strings.TrimSpace(track.VideoPath) != "" || strings.TrimSpace(track.VideoURL) != "")
	case SmartPlaylistFieldDurationSeconds:
		if track.DurationSeconds == nil {
			return smartPlaylistActual{missing: true, isNumber: true}
		}
		return smartPlaylistActual{numberValue: *track.DurationSeconds, isNumber: true}
	default:
		return smartPlaylistActual{missing: true}
	}
}

// SmartPlaylistMetadataMissing reports whether one of the core catalog
// identity fields is absent. It intentionally does not require enrichment,
// artwork, lyrics, or a network lookup.
func SmartPlaylistMetadataMissing(track TrackRecord) bool {
	return strings.TrimSpace(track.Title) == "" || strings.TrimSpace(track.Artist) == "" || strings.TrimSpace(track.Album) == "" || strings.TrimSpace(track.Genre) == "" || strings.TrimSpace(track.Year) == ""
}

func smartPlaylistTextActual(value string) smartPlaylistActual {
	value = strings.TrimSpace(value)
	return smartPlaylistActual{missing: value == "", textValue: value}
}

func smartPlaylistListActual(values []string) smartPlaylistActual {
	trimmed := trimSmartPlaylistValues(values)
	return smartPlaylistActual{missing: len(trimmed) == 0, textValues: trimmed}
}

func smartPlaylistBooleanActual(value bool) smartPlaylistActual {
	return smartPlaylistActual{booleanValue: value, isBoolean: true}
}

func smartPlaylistDateActual(value time.Time) smartPlaylistActual {
	return smartPlaylistActual{missing: value.IsZero(), dateValue: value, isDate: true}
}

func smartPlaylistYear(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if len(value) < 4 {
		return 0, false
	}
	year, err := strconv.Atoi(value[:4])
	if err != nil || year < 1 {
		return 0, false
	}
	return year, true
}

func trimSmartPlaylistValues(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func confidenceRank(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low":
		return 1
	case "medium", "moderate":
		return 2
	case "high":
		return 3
	default:
		return -1
	}
}

func isSmartPlaylistField(field SmartPlaylistField) bool {
	switch field {
	case SmartPlaylistFieldArtist, SmartPlaylistFieldAlbum, SmartPlaylistFieldTitle, SmartPlaylistFieldGenre, SmartPlaylistFieldMood, SmartPlaylistFieldYear, SmartPlaylistFieldDecade, SmartPlaylistFieldCreatedAt, SmartPlaylistFieldRecentlyAdded, SmartPlaylistFieldLastPlayedAt, SmartPlaylistFieldRecentlyPlayed, SmartPlaylistFieldPlayCount, SmartPlaylistFieldNeverPlayed, SmartPlaylistFieldConfidence, SmartPlaylistFieldMetadataConfidence, SmartPlaylistFieldEnrichmentConfidence, SmartPlaylistFieldMissingMetadata, SmartPlaylistFieldDuplicateCandidate, SmartPlaylistFieldVersion, SmartPlaylistFieldIncompleteAlbum, SmartPlaylistFieldHasLyrics, SmartPlaylistFieldHasTimedLyrics, SmartPlaylistFieldHasArtwork, SmartPlaylistFieldHasVideo, SmartPlaylistFieldDurationSeconds:
		return true
	default:
		return false
	}
}

func isSmartPlaylistOperator(op SmartPlaylistOperator) bool {
	switch op {
	case SmartPlaylistOperatorEquals, SmartPlaylistOperatorNotEquals, SmartPlaylistOperatorContains, SmartPlaylistOperatorIn, SmartPlaylistOperatorMissing, SmartPlaylistOperatorNotMissing, SmartPlaylistOperatorGreaterThan, SmartPlaylistOperatorGreaterOrEqual, SmartPlaylistOperatorLessThan, SmartPlaylistOperatorLessOrEqual, SmartPlaylistOperatorBetween, SmartPlaylistOperatorBefore, SmartPlaylistOperatorAfter, SmartPlaylistOperatorIsTrue, SmartPlaylistOperatorIsFalse:
		return true
	default:
		return false
	}
}

func isSmartPlaylistSortField(field SmartPlaylistSortField) bool {
	switch field {
	case SmartPlaylistSortTitle, SmartPlaylistSortArtist, SmartPlaylistSortAlbum, SmartPlaylistSortGenre, SmartPlaylistSortYear, SmartPlaylistSortCreatedAt, SmartPlaylistSortLastPlayedAt, SmartPlaylistSortPlayCount, SmartPlaylistSortDurationSeconds, SmartPlaylistSortID:
		return true
	default:
		return false
	}
}

func smartPlaylistNumericField(field SmartPlaylistField) bool {
	switch field {
	case SmartPlaylistFieldYear, SmartPlaylistFieldDecade, SmartPlaylistFieldPlayCount, SmartPlaylistFieldDurationSeconds:
		return true
	default:
		return false
	}
}

func smartPlaylistDateField(field SmartPlaylistField) bool {
	switch field {
	case SmartPlaylistFieldCreatedAt, SmartPlaylistFieldRecentlyAdded, SmartPlaylistFieldLastPlayedAt, SmartPlaylistFieldRecentlyPlayed:
		return true
	default:
		return false
	}
}

func smartPlaylistBooleanField(field SmartPlaylistField) bool {
	switch field {
	case SmartPlaylistFieldNeverPlayed, SmartPlaylistFieldMissingMetadata, SmartPlaylistFieldDuplicateCandidate, SmartPlaylistFieldIncompleteAlbum, SmartPlaylistFieldHasLyrics, SmartPlaylistFieldHasTimedLyrics, SmartPlaylistFieldHasArtwork, SmartPlaylistFieldHasVideo:
		return true
	default:
		return false
	}
}

func smartPlaylistOperatorAllowed(field SmartPlaylistField, op SmartPlaylistOperator) bool {
	if op == SmartPlaylistOperatorMissing || op == SmartPlaylistOperatorNotMissing {
		return true
	}
	if smartPlaylistBooleanField(field) {
		return op == SmartPlaylistOperatorIsTrue || op == SmartPlaylistOperatorIsFalse || op == SmartPlaylistOperatorEquals || op == SmartPlaylistOperatorNotEquals
	}
	if smartPlaylistDateField(field) {
		return op == SmartPlaylistOperatorEquals || op == SmartPlaylistOperatorNotEquals || op == SmartPlaylistOperatorBefore || op == SmartPlaylistOperatorAfter || op == SmartPlaylistOperatorBetween
	}
	if smartPlaylistNumericField(field) {
		return op == SmartPlaylistOperatorEquals || op == SmartPlaylistOperatorNotEquals || op == SmartPlaylistOperatorGreaterThan || op == SmartPlaylistOperatorGreaterOrEqual || op == SmartPlaylistOperatorLessThan || op == SmartPlaylistOperatorLessOrEqual || op == SmartPlaylistOperatorBetween
	}
	if field == SmartPlaylistFieldConfidence || field == SmartPlaylistFieldMetadataConfidence || field == SmartPlaylistFieldEnrichmentConfidence {
		return op == SmartPlaylistOperatorEquals || op == SmartPlaylistOperatorNotEquals || op == SmartPlaylistOperatorIn || op == SmartPlaylistOperatorGreaterOrEqual || op == SmartPlaylistOperatorLessOrEqual
	}
	return op == SmartPlaylistOperatorEquals || op == SmartPlaylistOperatorNotEquals || op == SmartPlaylistOperatorContains || op == SmartPlaylistOperatorIn
}
