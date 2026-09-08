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

func paintChatCommentsLayoutJS() string {
	return "if (window.workbenchApplyRegionVisible) { workbenchApplyRegionVisible('workbench-v2-chat', $workbench.regions.workbenchV2Chat.visible); workbenchApplyRegionVisible('workbench-v2-comments', $workbench.regions.workbenchV2Comments.visible) }; " +
		shareChatCommentsRatioJS() +
		"; if (window.workbenchReflow) { workbenchReflow() }; " +
		threadsLayoutReflowJS()
}

func ChatToggleClickAction() string {
	return "if ($workbench.regions.workbenchV2Chat.visible) { $workbench.regions.workbenchV2Chat.visible = false } else { $workbench.regions.workbenchV2Comments.visible = false; $workbench.regions.workbenchV2Chat.visible = true; " +
		commentsOpenCookieWriteJS(
			false,
		) + " }; " +
		paintChatCommentsLayoutJS()
}

func shareChatCommentsRatioJS() string {
	return `var chat=document.querySelector("[data-workbench-region='workbench-v2-chat']"); var comments=document.querySelector("[data-workbench-region='workbench-v2-comments']"); if (chat && comments) { if ($workbench.regions.workbenchV2Comments.visible) { comments.dataset.workbenchRatio = chat.dataset.workbenchRatio; $workbench.regions.workbenchV2Comments.ratio = $workbench.regions.workbenchV2Chat.ratio } else { chat.dataset.workbenchRatio = comments.dataset.workbenchRatio; $workbench.regions.workbenchV2Chat.ratio = $workbench.regions.workbenchV2Comments.ratio } }`
}

func CommentsToggleClickAction() string {
	return "$workbench.regions.workbenchV2Comments.visible = !$workbench.regions.workbenchV2Comments.visible; if ($workbench.regions.workbenchV2Comments.visible) { $workbench.regions.workbenchV2Chat.visible = false; " +
		commentsOpenCookieWriteJS(
			true,
		) + " } else { " + commentsOpenCookieWriteJS(
		false,
	) + " }; " +
		paintChatCommentsLayoutJS()
}
