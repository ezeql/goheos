package goheos

import (
	"regexp"
	"strconv"
	"strings"
)

// SearchCriteria describes a way a music source can be searched.
type SearchCriteria struct {
	Name        string
	CriteriaID  int
	Wildcard    bool
	ContainerID string
	Playable    bool
}

func searchCriteriaFromData(data payloadData) SearchCriteria {
	criteriaID, _ := data.intVal(attrSearchCriteriaID)
	return SearchCriteria{
		Name:        data.str(attrName),
		CriteriaID:  criteriaID,
		Wildcard:    data.str(attrWildcard) == valueYes,
		ContainerID: data.str(attrContainerID),
		Playable:    data.str(attrPlayable) == valueYes,
	}
}

// SearchResult is the result of a search in one music source.
type SearchResult struct {
	SourceID   int
	CriteriaID int
	Search     string
	Returned   int
	Count      int
	Items      []*MediaItem
}

func searchResultFromMessage(m *message) (*SearchResult, error) {
	sourceID, err := m.intValue(attrSourceID)
	if err != nil {
		return nil, err
	}
	criteriaID, err := m.intValue(attrSearchCriteriaID)
	if err != nil {
		return nil, err
	}
	returned, err := m.intValue(attrReturned)
	if err != nil {
		return nil, err
	}
	count, err := m.intValue(attrCount)
	if err != nil {
		return nil, err
	}
	payload, err := decodePayloadList(m.payload)
	if err != nil {
		return nil, err
	}
	items := make([]*MediaItem, 0, len(payload))
	for _, data := range payload {
		item, err := mediaItemFromData(data, sourceID, true, "")
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return &SearchResult{
		SourceID:   sourceID,
		CriteriaID: criteriaID,
		Search:     m.message[attrSearch],
		Returned:   returned,
		Count:      count,
		Items:      items,
	}, nil
}

// SearchStatistic is a per-source entry of a multi-search response: either a
// result count (Returned/Count) or an error (ErrorNumber).
type SearchStatistic struct {
	SourceID    int
	CriteriaID  int
	Returned    int
	Count       int
	ErrorNumber int
	IsError     bool
}

var tupleMatcher = regexp.MustCompile(`\(([0-9,-]+)\)`)

// parseSearchStatistics parses the stats/errno tuples of a multi-search:
// stats=(10,2,2,2),(1,0,57,57)  errno=(13,0,2),(8,0,-1061). Three-element
// tuples are errors, four-element tuples are statistics.
func parseSearchStatistics(data string) []SearchStatistic {
	var stats []SearchStatistic
	for _, match := range tupleMatcher.FindAllStringSubmatch(data, -1) {
		fields := strings.Split(match[1], ",")
		values := make([]int, len(fields))
		for i, f := range fields {
			values[i], _ = strconv.Atoi(f)
		}
		switch len(values) {
		case 3:
			stats = append(stats, SearchStatistic{
				SourceID:    values[0],
				CriteriaID:  values[1],
				ErrorNumber: values[2],
				IsError:     true,
			})
		case 4:
			stats = append(stats, SearchStatistic{
				SourceID:   values[0],
				CriteriaID: values[1],
				Returned:   values[2],
				Count:      values[3],
			})
		}
	}
	return stats
}

// MultiSearchResult is the result of a search across several music sources.
type MultiSearchResult struct {
	SourceIDs   []int
	CriteriaIDs []int
	Search      string
	Returned    int
	Count       int
	Items       []*MediaItem
	Statistics  []SearchStatistic
	Errors      []SearchStatistic
}

func multiSearchResultFromMessage(m *message) (*MultiSearchResult, error) {
	sourceIDsRaw, err := m.value(attrSourceID)
	if err != nil {
		return nil, err
	}
	criteriaIDsRaw, err := m.value(attrSearchCriteriaID)
	if err != nil {
		return nil, err
	}
	statsRaw, err := m.value(attrStats)
	if err != nil {
		return nil, err
	}
	errorsRaw, err := m.value(attrErrorNumber)
	if err != nil {
		return nil, err
	}
	returned, err := m.intValue(attrReturned)
	if err != nil {
		return nil, err
	}
	count, err := m.intValue(attrCount)
	if err != nil {
		return nil, err
	}
	payload, err := decodePayloadList(m.payload)
	if err != nil {
		return nil, err
	}

	statistics := parseSearchStatistics(statsRaw)
	// The payload is flat: each item's source is derived by walking the
	// per-source returned counts in order.
	var items []*MediaItem
	index := 0
	for _, stat := range statistics {
		for range stat.Returned {
			if index >= len(payload) {
				break
			}
			item, err := mediaItemFromData(payload[index], stat.SourceID, true, "")
			if err != nil {
				return nil, err
			}
			items = append(items, item)
			index++
		}
	}

	return &MultiSearchResult{
		SourceIDs:   splitInts(sourceIDsRaw),
		CriteriaIDs: splitInts(criteriaIDsRaw),
		Search:      m.message[attrSearch],
		Returned:    returned,
		Count:       count,
		Items:       items,
		Statistics:  statistics,
		Errors:      parseSearchStatistics(errorsRaw),
	}, nil
}

func splitInts(csv string) []int {
	if csv == "" {
		return nil
	}
	fields := strings.Split(csv, ",")
	values := make([]int, 0, len(fields))
	for _, f := range fields {
		n, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil {
			continue
		}
		values = append(values, n)
	}
	return values
}
