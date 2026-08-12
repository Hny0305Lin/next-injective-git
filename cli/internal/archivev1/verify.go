package archivev1

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

type snapshotEvidence struct {
	Schema string `json:"schema"`
	Source struct {
		ChainID   string `json:"chain_id"`
		Contract  string `json:"contract"`
		Height    string `json:"height"`
		BlockHash string `json:"block_hash"`
	} `json:"source"`
	Repositories []struct {
		Owner      string  `json:"owner"`
		Name       string  `json:"name"`
		ForkedFrom *string `json:"forked_from"`
	} `json:"repositories"`
	ModerationReports []struct {
		ID uint64 `json:"id"`
	} `json:"moderation_reports"`
	Usernames []struct {
		Name  string `json:"name"`
		Owner string `json:"owner"`
	} `json:"usernames"`
	BadgesByRecipient []struct {
		Recipient string `json:"recipient"`
	} `json:"badges_by_recipient"`
	Releases []struct {
		Version string `json:"version"`
	} `json:"releases"`
}

func VerifySnapshotInventory(snapshotPath string, inventory *Inventory) error {
	if err := ValidateInventory(inventory); err != nil {
		return err
	}
	raw, err := os.ReadFile(snapshotPath)
	if err != nil {
		return err
	}
	var snapshot snapshotEvidence
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&snapshot); err != nil {
		return fmt.Errorf("decode V1 snapshot: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("V1 snapshot contains trailing JSON data")
	}
	if snapshot.Schema != SnapshotSchema {
		return fmt.Errorf("unsupported V1 snapshot schema %q", snapshot.Schema)
	}
	height, err := strconv.ParseUint(snapshot.Source.Height, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid V1 snapshot height %q", snapshot.Source.Height)
	}
	if snapshot.Source.ChainID != inventory.ChainID || snapshot.Source.Contract != inventory.Contract || height != inventory.CutoverHeight {
		return errors.New("V1 snapshot source does not match inventory source")
	}
	if strings.ToLower(strings.TrimSpace(snapshot.Source.BlockHash)) != inventory.CutoverBlockHash {
		return errors.New("V1 snapshot cutover block hash does not match inventory")
	}

	repositories := make([]string, 0, len(snapshot.Repositories))
	forks := make(map[string]string, len(snapshot.Repositories))
	owners := map[string]struct{}{}
	for _, repository := range snapshot.Repositories {
		locator := repository.Owner + "/" + repository.Name
		repositories = append(repositories, locator)
		owners[repository.Owner] = struct{}{}
		if repository.ForkedFrom != nil {
			forks[locator] = *repository.ForkedFrom
		}
	}
	sort.Strings(repositories)
	inventoryRepositories := make([]string, len(inventory.Repositories))
	for index, repository := range inventory.Repositories {
		locator := repository.Owner + "/" + repository.Name
		inventoryRepositories[index] = locator
		if forks[locator] != repository.ForkedFrom {
			return fmt.Errorf("fork lineage mismatch for %s", locator)
		}
	}
	sort.Strings(inventoryRepositories)
	if !equalStringSlices(repositories, inventoryRepositories) {
		return fmt.Errorf("repository inventory mismatch: snapshot=%v inventory=%v", repositories, inventoryRepositories)
	}
	if !equalStringSlices(sortedKeys(owners), inventory.Owners) {
		return errors.New("repository owner inventory does not match snapshot")
	}

	reportIDs := make([]uint64, len(snapshot.ModerationReports))
	for index, report := range snapshot.ModerationReports {
		reportIDs[index] = report.ID
	}
	sort.Slice(reportIDs, func(i, j int) bool { return reportIDs[i] < reportIDs[j] })
	if !equalUint64Slices(reportIDs, inventory.ReportIDs) {
		return errors.New("moderation report inventory does not match snapshot")
	}
	usernames := make([]InventoryUsername, len(snapshot.Usernames))
	for index, username := range snapshot.Usernames {
		usernames[index] = InventoryUsername{Name: username.Name, Owner: username.Owner}
	}
	sort.Slice(usernames, func(i, j int) bool { return usernames[i].Name < usernames[j].Name })
	if !equalUsernames(usernames, inventory.Usernames) {
		return errors.New("username inventory does not match snapshot")
	}
	recipients := make([]string, len(snapshot.BadgesByRecipient))
	for index, group := range snapshot.BadgesByRecipient {
		recipients[index] = group.Recipient
	}
	sort.Strings(recipients)
	if !equalStringSlices(recipients, inventory.BadgeRecipients) {
		return errors.New("badge recipient inventory does not match snapshot")
	}
	versions := make([]string, len(snapshot.Releases))
	for index, release := range snapshot.Releases {
		versions[index] = release.Version
	}
	sort.Strings(versions)
	if !equalStringSlices(versions, inventory.ReleaseVersions) {
		return errors.New("release version inventory does not match snapshot")
	}
	return nil
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if strings.TrimSpace(left[index]) != strings.TrimSpace(right[index]) {
			return false
		}
	}
	return true
}

func equalUint64Slices(left, right []uint64) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equalUsernames(left, right []InventoryUsername) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
