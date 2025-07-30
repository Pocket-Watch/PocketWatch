import * as api from "./api.js";
import { getById, div, img, span, a, isSameDay } from "./util.js";

export { Chat }

const CHARACTER_LIMIT = 1000;

class Chat {
    constructor() {
        this.chatInput         = getById("chat_input_box");
        this.chatListRoot      = getById("chat_message_list_root");
        this.chatList          = getById("chat_message_list");
        this.sendMessageButton = getById("chat_send_button");
        this.contextMenu       = getById("chat_context_menu");
        this.contextMenuDelete = getById("chat_context_delete");

        this.isChatAtBottom = true;
        this.prevUserId     = -1;
        this.prevDate       = new Date();

        this.messages     = [];
        this.htmlMessages = [];

        this.contextMenuMessage     = null;
        this.contextMenuHtmlMessage = null;
    }

    hideContextMenu() {
        if (this.contextMenuHtmlMessage) {
            this.contextMenuHtmlMessage.classList.remove("highlight");
        }

        this.contextMenuMessage     = null;
        this.contextMenuHtmlMessage = null;
        hide(this.contextMenu);
    }

    showContextMenu(event, message, htmlMessage) {
        if (this.contextMenuHtmlMessage) {
            this.contextMenuHtmlMessage.classList.remove("highlight");
        }

        show(this.contextMenu);

        const msgRect  = htmlMessage.getBoundingClientRect();
        const chatRect = this.chatList.getBoundingClientRect();
        const height   = this.contextMenu.offsetHeight;
        const width    = this.contextMenu.offsetWidth;

        let contextMenuX = event.clientX;
        let protrusion = contextMenuX + width - msgRect.right;
        if (protrusion > 0) {
            contextMenuX -= protrusion;
        }

        let contextMenuY = event.clientY;
        protrusion = contextMenuY + height - chatRect.bottom;
        if (protrusion > 0) {
            contextMenuY -= protrusion;
        }

        this.contextMenu.style.left = (contextMenuX - msgRect.left) + "px";
        this.contextMenu.style.top  = (contextMenuY - chatRect.top)   + "px";

        this.contextMenuMessage     = message;
        this.contextMenuHtmlMessage = htmlMessage;

        this.contextMenuHtmlMessage.classList.add("highlight");
    }

    attachChatEvents() {
        this.contextMenu.oncontextmenu = event => {
            event.preventDefault();
            this.hideContextMenu();
        };

        this.contextMenuDelete.onclick = _ => api.chatDelete(this.contextMenuMessage.id);

        this.chatList.oncontextmenu = _ => { return false };
        document.addEventListener("click", _ => this.hideContextMenu());

        this.chatListRoot.onscroll = _ => {
            let root = this.chatListRoot;
            let scroll = root.scrollHeight - root.scrollTop - root.clientHeight;
            this.isChatAtBottom = Math.abs(scroll) < 60;
        };

        this.sendMessageButton.onclick = _ => {
            this.chatInput.focus();
            this.processMessageSendIntent();
        };

        // Handle shiftKey + Enter as 'new line' for formatting
        this.chatInput.onkeydown = event => {
            if (event.key === 'Enter' && !event.shiftKey) {
                event.preventDefault();
                this.processMessageSendIntent();
            }
        };

        window.addEventListener("resize", _ => this.keepAtBottom());
    }

    clear() {
        this.messages     = [];
        this.htmlMessages = [];

        this.prevUserId = -1;
        this.prevDate   = new Date(); 

        this.isChatAtBottom = true;

        while (this.chatList.lastChild) {
            this.chatList.removeChild(this.chatList.lastChild);
        }

        this.hideContextMenu();
    }

    findUser(userId, allUsers) {
        let user = allUsers.find(user => user.id === userId);

        if (!user) {
            const dummy = {
                id: 0,
                username: "Deleted user",
                avatar:   "img/default_avatar.png",
                online:   false,
            }

            user = dummy;
        }

        return user
    }

    createMessage(message, user) {
        let root      = div("chat_message")
        let avatar    = div("chat_message_avatar")
        let avatarImg = img(user.avatar)
        let right     = div("chat_message_right")
        let info      = div("chat_message_info")
        let username  = div("chat_message_username")
        let date      = div("chat_message_date")
        let text      = div("chat_message_text")

        let segments = this.linkify(message.message);
        username.textContent = user.username;

        if (user.id !== 0) {
            let color = Math.floor(Math.sin(user.id) * 10000);
            username.style.color = `hsl(${color} 70% 50%)`
        } 

        let d = new Date(message.unixTime);

        let Y = d.getFullYear() % 100;
        let M = d.getMonth().toString().padStart(2, "0");
        let D = d.getDate().toString().padStart(2, "0");

        let h = d.getHours().toString().padStart(2, "0");
        let m = d.getMinutes().toString().padStart(2, "0");

        let now = new Date();
        if (isSameDay(d, now)) {
            date.textContent = `${h}:${m}`;
        } else {
            date.textContent = `${Y}/${M}/${D}, ${h}:${m}`;
        }

        root.oncontextmenu = event => {
            event.preventDefault();

            if (this.contextMenuMessage && this.contextMenuMessage.id === message.id) {
                this.hideContextMenu();
            } else {
                this.showContextMenu(event, message, root);
            }
        };

        root.appendChild(avatar); {
            avatar.appendChild(avatarImg);
        }
        root.appendChild(right); {
            right.appendChild(info); {
                info.appendChild(username);
                info.appendChild(date);
            }
            right.appendChild(text); {
                text.append(...segments);
            }
        }

        return root
    }

    createSubMessage(message) {
        let root = div("chat_sub_message")
        let date = div("chat_sub_message_date")
        let text = div("chat_sub_message_text")

        let segments = this.linkify(message.message);

        let d = new Date(message.unixTime);
        let h = d.getHours().toString().padStart(2, "0");
        let m = d.getMinutes().toString().padStart(2, "0");

        date.textContent = `${h}:${m}`;

        root.oncontextmenu = event => {
            event.preventDefault();

            if (this.contextMenuMessage && this.contextMenuMessage.id === message.id) {
                this.hideContextMenu();
            } else {
                this.showContextMenu(event, message, root);
            }
        };

        root.appendChild(date);
        root.appendChild(text); {
            text.append(...segments);
        }

        return root;
    }

    linkify(content) {
        let segments = [];

        let segmentStart = 0;
        let parsingUrl   = false;

        for (let i = 0; i < content.length; i++) {
            let slice = content.slice(i)

            if (!parsingUrl && (slice.startsWith("http://") || slice.startsWith("https://"))) {
                if (segmentStart !== i) {
                    let text    = content.slice(segmentStart, i);
                    let segment = span(null, text)
                    segments.push(segment);
                }

                segmentStart = i;
                parsingUrl   = true;
            }

            let rune = content[i];
            if (rune === " " && parsingUrl) {
                let url     = content.slice(segmentStart, i);
                let segment = a(null, url)
                segments.push(segment);

                parsingUrl   = false;
                segmentStart = i;;
            }
        }

        if (parsingUrl) {
            let url     = content.slice(segmentStart);
            let segment = a(null, url)
            segments.push(segment);
        } else {
            let text    = content.slice(segmentStart);
            let segment = span(null, text)
            segments.push(segment);
        }

        return segments
    }

    addMessage(chatMsg, allUsers) {
        let user = this.findUser(chatMsg.authorId, allUsers)
        let date = new Date(chatMsg.unixTime);

        let message;
        if (this.prevUserId !== user.id || !isSameDay(this.prevDate, date)) {
            message = this.createMessage(chatMsg, user);
        } else {
            message = this.createSubMessage(chatMsg, user);
        }

        this.prevUserId = user.id;
        this.prevDate   = date;

        this.messages.push(chatMsg);
        this.htmlMessages.push(message);
        this.chatList.appendChild(message);

        this.keepAtBottom();
    }

    removeMessageById(messageId, allUsers) {
        let index = this.messages.findIndex(message => message.id === messageId);
        if (index === -1) {
            console.warn("WARN: Chat::remove failed. Failed to find message with ID =", messageId);
            return;
        }

        let message = this.messages[index];
        let htmlMessage = this.htmlMessages[index];

        if (index === this.messages.length - 1) {
            let prev = this.messages[index - 1];

            if (prev) {
                this.prevUserId = prev.authorId;
                this.prevDate   = new Date(prev.unixTime);
            } else {
                this.prevUserId = -1;
                this.prevDate   = new Date();
            }
        }

        if (htmlMessage.classList.contains("chat_message")) {
            let next     = this.messages[index + 1];
            let nextHtml = this.htmlMessages[index + 1];
            if (next && !nextHtml.classList.contains("chat_message")) {
                let user = this.findUser(next.authorId, allUsers)
                let newHtml = this.createMessage(next, user);

                if (this.contextMenuMessage && this.contextMenuMessage.id === next.id) {
                    this.contextMenuHtmlMessage = newHtml;
                    newHtml.classList.add("highlight");
                }

                this.htmlMessages[index + 1] = newHtml;
                this.chatList.replaceChild(newHtml, nextHtml);
            }
        }

        if (this.contextMenuMessage && this.contextMenuMessage.id === message.id) {
            this.hideContextMenu();
        }

        this.messages.splice(index, 1);
        this.htmlMessages.splice(index, 1);
        this.chatList.removeChild(htmlMessage);
    }

    processMessageSendIntent() {
        let content = this.chatInput.value;
        if (content.length === 0 || content.length > CHARACTER_LIMIT) {
            console.warn("WARN: Message is empty or exceeds", CHARACTER_LIMIT, "characters");
            // This is handled server side for length
            return;
        }

        api.chatSend(content)
        this.chatInput.value = "";
    }

    loadMessages(messages, allUsers) {
        for (let i = 0; i < messages.length; i++) {
            this.addMessage(messages[i], allUsers);
        }

        this.keepAtBottom();
    }

    keepAtBottom() {
        if (this.isChatAtBottom) {
            this.chatListRoot.scrollTo(0, this.chatList.scrollHeight)
        }
    }
} 

/*type ChatMessage struct {
    Message  string `json:"message"`
    UnixTime int64  `json:"unixTime"`
    Id       uint64 `json:"id"`
    AuthorId uint64 `json:"authorId"`
    Edited   bool   `json:"edited"`
}*/
