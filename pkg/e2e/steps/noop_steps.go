package steps

import "testing"

func AuthenticatedAs(t testing.TB, _ any, _ string) { t.Helper() }
func LoadFixture(t testing.TB, _ any, _ string)     { t.Helper() }
func Visit(t testing.TB, _ any, _ string)           { t.Helper() }
func OpenPlanWorkspace(t testing.TB, _ any, _ string) {
	t.Helper()
}
func OpenWorkspaceChat(t testing.TB, _ any, _ string) { t.Helper() }
func OpenFreeformChatFixture(t testing.TB, _ any, _ string) {
	t.Helper()
}
func OpenThoughtsRootChat(t testing.TB, _ any, _ string) { t.Helper() }
func SendFreeformChatPrompt(t testing.TB, _ any, _ string) {
	t.Helper()
}
func WaitForLatestFreeformChatRun(t testing.TB, _ any, _ string) {
	t.Helper()
}
func WaitForLatestFreeformChatRunCompletion(t testing.TB, _ any, _ string) {
	t.Helper()
}
func SeedLatestWorkspaceChats(t testing.TB, _ any, _, _ string) {
	t.Helper()
}
func OpenSeededWorkspaceChat(t testing.TB, _ any, _ string) { t.Helper() }
func ReloadChat(t testing.TB, _ any, _ string)              { t.Helper() }
func ReopenCurrentChat(t testing.TB, _ any, _ string)       { t.Helper() }
func RememberFileHash(t testing.TB, _ any, _ string)        { t.Helper() }
func SendPiDocsReviewPrompt(t testing.TB, _ any, _, _ string) {
	t.Helper()
}
func WaitForChatMarker(t testing.TB, _ any, _ string) { t.Helper() }
func WaitForFeatureReady(t testing.TB, _ any, _ string) {
	t.Helper()
}
func ExpectRegionVisible(t testing.TB, _ any, _ string) { t.Helper() }
func ExpectRegionReachable(t testing.TB, _ any, _ string) {
	t.Helper()
}
func ExpectTabSelected(t testing.TB, _ any, _ string) { t.Helper() }
func ExpectTextAbsent(t testing.TB, _ any, _ string)  { t.Helper() }
func ExpectTranscriptContains(t testing.TB, _ any, _ string) {
	t.Helper()
}
func ExpectFileHashChanged(t testing.TB, _ any, _ string) { t.Helper() }
func ExpectPiReviewFileSections(t testing.TB, _ any, _ string) {
	t.Helper()
}
func ExpectOnlyFileChanged(t testing.TB, _ any, _ string) { t.Helper() }
