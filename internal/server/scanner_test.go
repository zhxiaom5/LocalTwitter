package server

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseAwemeIDAndMediaType(t *testing.T) {
	name := "2026-04-08_WHToaPYvsw8d0-eZ_打桩外翻体育生弟弟.jpg"
	if got := ParseAwemeID(name); got != "WHToaPYvsw8d0-eZ" {
		t.Fatalf("ParseAwemeID() = %q", got)
	}
	multiline := "2025-07-16_Ywzgc9OZIg6SSiYp_喂食肌肉男\nhttpfansone.co.mp4"
	if got := ParseAwemeID(multiline); got != "Ywzgc9OZIg6SSiYp" {
		t.Fatalf("ParseAwemeID(multiline) = %q", got)
	}
	if got := ParseTitle(multiline); got != "喂食肌肉男\nhttpfansone.co" {
		t.Fatalf("ParseTitle(multiline) = %q", got)
	}
	if got := ParseTitle(name); got != "打桩外翻体育生弟弟" {
		t.Fatalf("ParseTitle() = %q", got)
	}
	if got := MediaType(name); got != "" {
		t.Fatalf("MediaType(image) = %q", got)
	}
	if got := MediaType("clip.mp4"); got != "video" {
		t.Fatalf("MediaType(video) = %q", got)
	}
	if got := ParseAwemeID("2026-04-25__THpK5EgXgKSXZCT_收到绮趣家.json"); got != "_THpK5EgXgKSXZCT" {
		t.Fatalf("ParseAwemeID(leading underscore) = %q", got)
	}
}

func TestValidateDirectoryRejectsFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDirectory(dir); err != nil {
		t.Fatalf("directory should validate: %v", err)
	}
	if err := ValidateDirectory(file); err == nil {
		t.Fatal("file path should be rejected")
	}
}

func TestFileBrowserListsOnlyVisibleDirectories(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "creator"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".hidden"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := (FileBrowser{}).List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "creator" {
		t.Fatalf("unexpected entries: %#v", entries)
	}
}

func TestAccessiblePathsAllowLocalDevelopmentWhenUnset(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !PathAllowed("/any/path", nil) {
		t.Fatal("empty accessible paths should allow local development paths")
	}
	if err := ValidateAccessibleDirectory(dir, nil); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAccessibleDirectory(file, nil); err == nil {
		t.Fatal("file path should still be rejected")
	}
}

func TestFileBrowserRestrictsRootsAndListToAccessiblePaths(t *testing.T) {
	root := t.TempDir()
	allowedA := filepath.Join(root, "allowed-a")
	allowedB := filepath.Join(root, "allowed-b")
	child := filepath.Join(allowedA, "creator")
	blocked := filepath.Join(root, "blocked")
	for _, dir := range []string{allowedA, allowedB, child, blocked} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	browser := FileBrowser{allowedPaths: ParseAccessiblePaths(allowedA + string(os.PathListSeparator) + allowedB)}
	roots := browser.Roots()
	if len(roots) != 2 {
		t.Fatalf("roots = %#v", roots)
	}
	entries, err := browser.List(allowedA)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Path != child {
		t.Fatalf("entries = %#v", entries)
	}
	if _, err := browser.List(blocked); !errors.Is(err, ErrPathNotAllowed) {
		t.Fatalf("blocked err = %v, want ErrPathNotAllowed", err)
	}
}

func TestAccessibleDirectoryRejectsSiblingPrefix(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "media")
	sibling := filepath.Join(root, "media-other")
	if err := os.Mkdir(allowed, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(sibling, 0o755); err != nil {
		t.Fatal(err)
	}
	if !PathAllowed(filepath.Join(allowed, "child"), []string{allowed}) {
		t.Fatal("child should be allowed")
	}
	if PathAllowed(sibling, []string{allowed}) {
		t.Fatal("sibling sharing path prefix should not be allowed")
	}
}

func TestReadCreatorDirsResolvesSupportedLayouts(t *testing.T) {
	root := t.TempDir()
	layouts := map[string]string{
		"nested": filepath.Join(root, "nested", "nested", "video"),
		"video":  filepath.Join(root, "video", "video"),
		"direct": filepath.Join(root, "direct"),
	}
	for _, path := range layouts {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	dirs, err := readCreatorDirs(root)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, dir := range dirs {
		got[dir.Name] = dir.Path
	}
	for name, want := range layouts {
		if got[name] != want {
			t.Fatalf("%s path = %q, want %q", name, got[name], want)
		}
	}
}

func TestReadCreatorDirsScansNestedAndFlatVideoDirs(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "dahuoluowan", "dahuoluowan", "video")
	flat := filepath.Join(root, "dahuoluowan", "video")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(flat, 0o755); err != nil {
		t.Fatal(err)
	}
	dirs, err := readCreatorDirs(root)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, dir := range dirs {
		if dir.Name == "dahuoluowan" {
			got = append(got, dir.Path)
		}
	}
	if len(got) != 2 {
		t.Fatalf("paths = %#v, want both nested and flat video dirs", got)
	}
	want := map[string]bool{nested: true, flat: true}
	for _, path := range got {
		if !want[path] {
			t.Fatalf("unexpected path %q in %#v", path, got)
		}
	}
}

func TestScanCreatorParsesMetadataAndMedia(t *testing.T) {
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "2026-04-08_WHToaPYvsw8d0-eZ_测试.json")
	if err := os.WriteFile(jsonPath, []byte(`{"ID":"2041001","Text":"会好 #测试","Username":"creator","Name":"作者","TimeParsed":"2026-04-08T10:00:00Z","PermanentURL":"https://twitter.com/creator/status/2041001","Mentions":[{"Username":"friend","Name":"朋友"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2026-04-08_WHToaPYvsw8d0-eZ_测试.jpg"), []byte("img"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2026-04-08_WHToaPYvsw8d0-eZ_测试.mp4"), []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2026-04-08_WHToaPYvsw8d0-eZ_测试.nfo"), []byte("nfo"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2026-04-08_WHToaPYvsw8d0-eZ_测试.ass"), []byte("ass"), 0o644); err != nil {
		t.Fatal(err)
	}
	works, err := scanCreator(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(works) != 1 {
		t.Fatalf("works len = %d", len(works))
	}
	work := works[0]
	if work.AwemeID != "2041001" || work.Description != "会好 #测试" || work.SourceURL == "" || len(work.Media) != 1 || work.Media[0].Type != "video" {
		t.Fatalf("unexpected work: %#v", work)
	}
	if work.CoverFile == "" || filepath.Base(work.CoverFile) != "2026-04-08_WHToaPYvsw8d0-eZ_测试.jpg" {
		t.Fatalf("cover file should be indexed from same-prefix image: %#v", work)
	}
	if _, err := time.Parse(time.RFC3339, work.PublishedAt); err != nil {
		t.Fatalf("published_at not RFC3339: %q", work.PublishedAt)
	}
}

func TestScanCreatorRepairsMP4Faststart(t *testing.T) {
	dir := t.TempDir()
	videoPath := filepath.Join(dir, "2026-04-08_WHToaPYvsw8d0-eZ_后置moov.mp4")
	if err := os.WriteFile(videoPath, testMP4WithMoovAfterMdat(), 0o644); err != nil {
		t.Fatal(err)
	}

	works, err := scanCreator(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(works) != 1 || len(works[0].Media) != 1 {
		t.Fatalf("works = %#v, want one video work", works)
	}
	repaired, err := os.ReadFile(videoPath)
	if err != nil {
		t.Fatal(err)
	}
	moov := bytes.Index(repaired, []byte("moov"))
	mdat := bytes.Index(repaired, []byte("mdat"))
	if moov < 0 || mdat < 0 || moov > mdat {
		t.Fatalf("mp4 should be faststart after scan, moov=%d mdat=%d bytes=%q", moov, mdat, repaired)
	}
	if !bytes.Contains(repaired, []byte{0, 0, 0, 85}) {
		t.Fatalf("stco chunk offset should be adjusted by moved moov size, bytes=%q", repaired)
	}
}

func TestScanCreatorWarnsWhenVideoRepairFailsButKeepsWork(t *testing.T) {
	dir := t.TempDir()
	videoPath := filepath.Join(dir, "2026-04-08_WHToaPYvsw8d0-eZ_异常视频.mp4")
	if err := os.WriteFile(videoPath, []byte("not an mp4"), 0o644); err != nil {
		t.Fatal(err)
	}

	works, warnings, err := scanCreatorWithWarnings(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(works) != 1 || len(works[0].Media) != 1 {
		t.Fatalf("work should still be indexed after repair warning: %#v", works)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "视频兼容处理失败") {
		t.Fatalf("warnings = %#v, want video repair warning", warnings)
	}
}

func TestScanCreatorWithoutVideoRepairDoesNotRewriteVideo(t *testing.T) {
	dir := t.TempDir()
	videoPath := filepath.Join(dir, "2026-04-08_WHToaPYvsw8d0-eZ_后置moov.mp4")
	original := testMP4WithMoovAfterMdat()
	if err := os.WriteFile(videoPath, original, 0o644); err != nil {
		t.Fatal(err)
	}

	works, err := scanCreatorWithoutVideoRepair(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(works) != 1 || len(works[0].Media) != 1 {
		t.Fatalf("work should be indexed without repair: %#v", works)
	}
	got, err := os.ReadFile(videoPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatal("update scan should not rewrite existing video while playback may be active")
	}
}

func TestScanCreatorDoesNotWarnForFaststartMP4WithTruncatedMdat(t *testing.T) {
	dir := t.TempDir()
	videoPath := filepath.Join(dir, "2026-04-08_WHToaPYvsw8d0-eZ_前置moov.mp4")
	original := testMP4WithMoovBeforeInvalidMdat()
	if err := os.WriteFile(videoPath, original, 0o644); err != nil {
		t.Fatal(err)
	}

	works, warnings, err := scanCreatorWithWarnings(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(works) != 1 || len(works[0].Media) != 1 {
		t.Fatalf("work should be indexed: %#v", works)
	}
	if len(warnings) != 0 {
		t.Fatalf("already-faststart truncated mdat should not warn, got %#v", warnings)
	}
	got, err := os.ReadFile(videoPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatal("already-faststart file should not be rewritten")
	}
}

func TestRepairMP4FaststartNoopsWhenAlreadyCompatible(t *testing.T) {
	dir := t.TempDir()
	videoPath := filepath.Join(dir, "clip.mp4")
	original := testMP4WithMoovBeforeMdat()
	if err := os.WriteFile(videoPath, original, 0o644); err != nil {
		t.Fatal(err)
	}

	repaired, err := ensureVideoSeekable(videoPath)
	if err != nil {
		t.Fatal(err)
	}
	if repaired {
		t.Fatal("already-faststart MP4 should not be rewritten")
	}
	got, err := os.ReadFile(videoPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatal("already-compatible MP4 changed unexpectedly")
	}
}

func testMP4WithMoovAfterMdat() []byte {
	return joinMP4Boxes(
		mp4TestBox("ftyp", []byte("isom\x00\x00\x02\x00isomiso2mp41")),
		mp4TestBox("mdat", []byte("video-payload")),
		mp4TestBox("moov", testMoovPayload(25)),
	)
}

func testMP4WithMoovBeforeMdat() []byte {
	return joinMP4Boxes(
		mp4TestBox("ftyp", []byte("isom\x00\x00\x02\x00isomiso2mp41")),
		mp4TestBox("moov", testMoovPayload(25)),
		mp4TestBox("mdat", []byte("video-payload")),
	)
}

func testMP4WithMoovBeforeInvalidMdat() []byte {
	return joinMP4Boxes(
		mp4TestBox("ftyp", []byte("isom\x00\x00\x02\x00isomiso2mp41")),
		mp4TestBox("moov", testMoovPayload(25)),
		mp4TestBoxWithDeclaredSize("mdat", 10_000, []byte("short")),
	)
}

func joinMP4Boxes(boxes ...[]byte) []byte {
	return bytes.Join(boxes, nil)
}

func mp4TestBox(name string, payload []byte) []byte {
	size := uint32(len(payload) + 8)
	box := make([]byte, size)
	box[0] = byte(size >> 24)
	box[1] = byte(size >> 16)
	box[2] = byte(size >> 8)
	box[3] = byte(size)
	copy(box[4:8], name)
	copy(box[8:], payload)
	return box
}

func mp4TestBoxWithDeclaredSize(name string, declaredSize uint32, payload []byte) []byte {
	box := make([]byte, len(payload)+8)
	binary.BigEndian.PutUint32(box[0:4], declaredSize)
	copy(box[4:8], name)
	copy(box[8:], payload)
	return box
}

func testMoovPayload(chunkOffset uint32) []byte {
	stcoPayload := make([]byte, 12)
	binary.BigEndian.PutUint32(stcoPayload[4:8], 1)
	binary.BigEndian.PutUint32(stcoPayload[8:12], chunkOffset)
	return mp4TestBox("trak", mp4TestBox("mdia", mp4TestBox("minf", mp4TestBox("stbl", mp4TestBox("stco", stcoPayload)))))
}
