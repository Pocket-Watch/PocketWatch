import * as api from "./api.js";
import { getById, div, img, isSameDay } from "./util.js";

export { Chat }

const CHARACTER_LIMIT = 1000;

class Chat {
    constructor() {
        this.chatInput         = getById("chat_input_box");
        this.chatArea          = getById("chat_text_content");
        this.sendMessageButton = getById("chat_send_button");

        this.isChatAtBottom = true;
        this.prevUserId     = -1;
        this.prevDate       = new Date();

        this.messages     = [];
        this.htmlMessages = [];
    }

    attachChatEvents() {
        this.chatArea.onscroll = _ => {
            let scroll = this.chatArea.scrollHeight - this.chatArea.scrollTop - this.chatArea.clientHeight;
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

        while (this.chatArea.lastChild) {
            this.chatArea.removeChild(this.chatArea.lastChild);
        }
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

        text.textContent     = message.message;
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

        root.appendChild(avatar); {
            avatar.appendChild(avatarImg);
        }
        root.appendChild(right); {
            right.appendChild(info); {
                info.appendChild(username);
                info.appendChild(date);
            }
            right.appendChild(text);
        }

        return root
    }

    createSubMessage(message) {
        let root = div("chat_sub_message")
        let date = div("chat_sub_message_date")
        let text = div("chat_sub_message_text")

        text.textContent = message.message;

        let d = new Date(message.unixTime);
        let h = d.getHours().toString().padStart(2, "0");
        let m = d.getMinutes().toString().padStart(2, "0");

        date.textContent = `${h}:${m}`;

        root.appendChild(date);
        root.appendChild(text);

        return root;
    }

    linkify(content) {
        let string = "This is a https://random.org test message.";

        let s1 = "<span>This is a</span>";
        let s2 = "<a>https://random.org</a>";
        let s3 = "<span>test message.</span>";

        // Combining string, slice, startsWith
    }

    addMessage(chatMsg, allUsers) {
        let index = allUsers.findIndex(user => user.id === chatMsg.authorId);
        let user  = allUsers[index];

        if (!user) {
            const dummy = {
                id: 0,
                username: "Deleted user",
                avatar:   "img/default_avatar.png",
                online:   false,
            }

            user = dummy;
        }

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
        this.chatArea.appendChild(message);

        this.keepAtBottom();
    }

    removeMessageById(messageId) {
        let index = this.messages.findIndex(message => message.id = messageId);
        if (index === -1) {
            console.warn("WARN: Chat::remove failed. Failed to find message with ID =", messageId);
            return;
        }

        // let message     = this.messages[index];
        // let htmlMessage = this.htmlMessages[index];

        if (index === this.messages.length - 1) {
            let prev = this.messages[index - 1];
            this.prevUserId = prev.authorId;
            this.prevDate   = new Date(prev.unixTime);
        }

        // if delete message was a header message, the next one also must be a header message
        // and it its not, turn it into a header message

        this.messages.splice(index, 1);
        this.htmlMessages.splice(index, 1);
        this.chatArea.removeChild(htmlEntry);
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
            this.chatArea.scrollTo(0, this.chatArea.scrollHeight)
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
