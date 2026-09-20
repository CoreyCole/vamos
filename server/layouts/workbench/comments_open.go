package workbench

import (
	"net/http"
)

// CommentsOpenCookie stores comments-pane visibility across sibling GETs.
// Missing/invalid => closed.
const CommentsOpenCookie = "wb2_comments_open"

// CommentsOpenFromRequest reads wb2_comments_open; missing/invalid => closed.
func CommentsOpenFromRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	c, err := r.Cookie(CommentsOpenCookie)
	if err != nil || (c.Value != "0" && c.Value != "1") {
		return false
	}
	return c.Value == "1"
}

func commentsOpenCookieWriteJS(open bool) string {
	v := "0"
	if open {
		v = "1"
	}
	return "try { document.cookie = '" + CommentsOpenCookie + "=" + v +
		"; path=/; SameSite=Lax; Max-Age=31536000'; sessionStorage.setItem(" +
		"'workbench-v2:comments-open', '" + v + "') } catch (e) {}"
}

// CommentsCloseCookieJS clears the comments cookie so a View Chat GET can open chat.
func CommentsCloseCookieJS() string {
	return commentsOpenCookieWriteJS(false)
}

func hideThreadsWhenChatAndCommentsClosedJS() string {
	return "if (!$workbench.regions.workbenchV2Chat.visible && !$workbench.regions.workbenchV2Comments.visible) { $workbench.regions.workbenchV2Threads.visible = false; " +
		threadsOpenCookieWriteJS(
			false,
		) + " }"
}

func paintChatCommentsLayoutJS() string {
	return hideThreadsWhenChatAndCommentsClosedJS() +
		"; if (window.workbenchApplyRegionVisible) { workbenchApplyRegionVisible('workbench-v2-chat', $workbench.regions.workbenchV2Chat.visible); workbenchApplyRegionVisible('workbench-v2-comments', $workbench.regions.workbenchV2Comments.visible); workbenchApplyRegionVisible('workbench-v2-threads', $workbench.regions.workbenchV2Threads.visible) }; " +
		shareChatCommentsRatioJS() +
		"; if (window.workbenchReflow) { workbenchReflow() }; " +
		threadsLayoutReflowJS()
}

func ChatToggleClickAction() string {
	return "if ($workbench.regions.workbenchV2Chat.visible) { $workbench.regions.workbenchV2Chat.visible = false; " +
		chatOpenCookieWriteJS(
			false,
		) +
		" } else { $workbench.regions.workbenchV2Comments.visible = false; $workbench.regions.workbenchV2Chat.visible = true; $workbench.activeRegionID = 'workbenchV2Chat'; el.closest('#workbench-root').dataset.workbenchMobileActive = 'workbenchV2Chat'; " +
		commentsOpenCookieWriteJS(
			false,
		) + "; " +
		chatOpenCookieWriteJS(
			true,
		) +
		" }; " +
		paintChatCommentsLayoutJS()
}

func shareChatCommentsRatioJS() string {
	return `var chat=document.querySelector("[data-workbench-region='workbench-v2-chat']"); var comments=document.querySelector("[data-workbench-region='workbench-v2-comments']"); if (chat && comments) { var r=$workbench.regions.workbenchV2Comments.visible ? $workbench.regions.workbenchV2Comments.ratio : $workbench.regions.workbenchV2Chat.ratio; if (r == null || r === 0) { r=Number(($workbench.regions.workbenchV2Comments.visible ? comments : chat).dataset.workbenchRatio || 0) } chat.dataset.workbenchRatio=r; comments.dataset.workbenchRatio=r; $workbench.regions.workbenchV2Chat.ratio=r; $workbench.regions.workbenchV2Comments.ratio=r }`
}

func CommentsToggleClickAction() string {
	return "$workbench.regions.workbenchV2Comments.visible = !$workbench.regions.workbenchV2Comments.visible; if ($workbench.regions.workbenchV2Comments.visible) { $workbench.activeRegionID = 'workbenchV2Comments'; el.closest('#workbench-root').dataset.workbenchMobileActive = 'workbenchV2Comments'; $workbench.regions.workbenchV2Chat.visible = false; " +
		commentsOpenCookieWriteJS(
			true,
		) + " } else { " + commentsOpenCookieWriteJS(
		false,
	) + " }; " +
		paintChatCommentsLayoutJS()
}
