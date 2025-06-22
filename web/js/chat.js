import * as api from "./api.js";
import { getById, div, img, isSameDay } from "./util.js";

export { Chat }

const CHARACTER_LIMIT = 1000;

class Chat {
    constructor() {
        this.chatInput         = getById("chat_input_box");
        this.chatArea          = getById("chat_text_content");
        this.sendMessageButton = getById("chat_send_button");

        this.prevUserId = 0;
        this.attachListeners();
    }

    clear() {
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
        let color     = Math.floor(Math.sin(user.id) * 10000);
        let date      = div("chat_message_date")
        let text      = div("chat_message_text")

        text.textContent     = message.message;
        username.textContent = user.username;
        username.style.color = `hsl(${color} 70% 50%)`

        let d = new Date(message.unixTime);

        let Y = d.getFullYear() % 100;
        let M = d.getMonth().toString().padStart(2, "0");
        let D = d.getDate().toString().padStart(2, "0");

        let h = d.getHours().toString().padStart(2, "0");
        let m = d.getMinutes().toString().padStart(2, "0");

        if (isSameDay(d)) {
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
        root.textContent = message.message;
        return root;
    }

    addMessage(chatMsg, allUsers) {
        let index = allUsers.findIndex(user => user.id === chatMsg.authorId);
        let user = allUsers[index];

        if (!user) {
            const dummy = {
                id: 0,
                username: "Deleted user",
                avatar: "img/default_avatar.png",
                online: false,
            }

            user = dummy;
        }

        let message;
        if (this.prevUserId !== user.id) {
            message = this.createMessage(chatMsg, user);
        } else {
            message = this.createSubMessage(chatMsg, user);
        }

        this.chatArea.appendChild(message);

        let scroll = Math.abs(this.chatArea.scrollHeight - this.chatArea.scrollTop - this.chatArea.clientHeight);
        if (scroll < 60) {
            this.scrollToBottom();
        }

        this.prevUserId = user.id;
    }

    attachListeners() {
        this.sendMessageButton.addEventListener("click", _ => {
            this.processMessageSendIntent()
        })

        // Handle shiftKey + Enter as 'new line' for formatting
        this.chatInput.addEventListener('keydown', e => {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                this.processMessageSendIntent()
            }
        });
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

        this.scrollToBottom();
    }

    scrollToBottom() {
        this.chatArea.scrollTo(0, this.chatArea.scrollHeight)
    }
} 

/*type ChatMessage struct {
    Message  string `json:"message"`
    UnixTime int64  `json:"unixTime"`
    Id       uint64 `json:"id"`
    AuthorId uint64 `json:"authorId"`
    Edited   bool   `json:"edited"`
}*/
