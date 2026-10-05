package representation

import (
	"math"
	"strconv"
	"strings"
)

// Negotiate picks a claimed Accept media type for row data. An empty Accept,
// application/json, application/vnd.pgrst.array+json, and */* claim the JSON
// array. The singular object and CSV are claimed. The claimed type with the
// highest quality wins; on equal quality the first one wins. With no
// acceptable claimed type, Negotiate refuses with UnsupportedMedia.
func Negotiate(acceptHeaders []string) (Spec, error) {
	preferences := mediaPreferences(acceptHeaders)
	if len(preferences) == 0 {
		return Default(), nil
	}

	var selected Spec
	quality := -1.0
	for _, preference := range preferences {
		if preference.quality == 0 {
			continue
		}
		if chosen, ok := claim(preference.mediaType); ok && preference.quality > quality {
			selected = chosen
			quality = preference.quality
		}
	}
	if quality >= 0 {
		return selected, nil
	}
	return Spec{}, Refuse(acceptHeaders)
}

type mediaPreference struct {
	mediaType string
	quality   float64
}

func mediaPreferences(headers []string) []mediaPreference {
	var preferences []mediaPreference
	for _, header := range headers {
		for _, part := range strings.Split(header, ",") {
			mediaType := mediaTypeOf(part)
			if mediaType == "" {
				continue
			}
			preferences = append(preferences, mediaPreference{
				mediaType: mediaType,
				quality:   qualityOf(part),
			})
		}
	}
	return preferences
}

// mediaTypeOf is the lower-case media type of one Accept member.
func mediaTypeOf(part string) string {
	mediaType, _, _ := strings.Cut(part, ";")
	return strings.ToLower(strings.TrimSpace(mediaType))
}

// qualityOf is the q parameter of one Accept member. A missing q is 1. A q
// that is not a number from 0 to 1 makes the member not acceptable.
func qualityOf(part string) float64 {
	parameters := strings.Split(part, ";")[1:]
	quality := 1.0
	for _, parameter := range parameters {
		name, value, hasValue := strings.Cut(strings.TrimSpace(parameter), "=")
		if !hasValue || !strings.EqualFold(strings.TrimSpace(name), "q") {
			continue
		}
		parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil || math.IsNaN(parsed) || parsed < 0 || parsed > 1 {
			return 0
		}
		quality = parsed
	}
	return quality
}

func claim(mediaType string) (Spec, bool) {
	switch mediaType {
	case MediaJSON, "*/*":
		return Default(), true
	case MediaArrayJSON, "application/vnd.pgrst.array":
		return Spec{Kind: KindJSONArray, ContentType: MediaArrayJSON}, true
	case MediaObjectJSON, "application/vnd.pgrst.object":
		return Spec{Kind: KindSingularObject, ContentType: MediaObjectJSON}, true
	case MediaCSV:
		return Spec{Kind: KindCSV, ContentType: MediaCSVCharset}, true
	default:
		return Spec{}, false
	}
}
