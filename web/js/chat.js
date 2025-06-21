import * as api from "./api.js";
import { getById, div, dynamicImg, img } from "./util.js";

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

    createHeaderMessage(message, user) {
        let root     = div("chat_message_header")
        let avatar   = div("chat_message_header_avatar")
        // let img      = dynamicImg(user.avatar)
        let avatarImg = img(user.avatar)
        let right    = div("chat_message_header_right")
        let info     = div("chat_message_header_info")
        let username = div("chat_message_header_username")
        let color    = Math.floor(Math.sin(user.id) * 10000);
        let date     = div("chat_message_header_date")
        let text     = div("chat_message_header_text")

        text.textContent     = message.message;
        username.textContent = user.username;
        username.style.color = `hsl(${color} 70% 50%)`

        let d = new Date(message.unixTime);

        let Y = d.getFullYear() % 100;
        let M = d.getMonth().toString().padStart(2, "0");
        let D = d.getUTCDay().toString().padStart(2, "0");

        let h = d.getHours().toString().padStart(2, "0");
        let m = d.getMinutes().toString().padStart(2, "0");

        date.textContent = `${Y}/${M}/${D}, ${h}:${m}`;

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

    createTextMessage(message, user) {

    }

    addMessage(chatMsg, allUsers) {
        let chatDiv = document.createElement("div");
        let index = allUsers.findIndex(user => user.id === chatMsg.authorId);
        let user = allUsers[index];

        let username = "Deleted user"
        if (user) {
            username = user.username;
        }

        let message = this.createHeaderMessage(chatMsg, user);

        this.chatArea.appendChild(message);
        this.chatArea.scrollTo(0, this.chatArea.scrollHeight)
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
    }
}

/*type ChatMessage struct {
    Message  string `json:"message"`
    UnixTime int64  `json:"unixTime"`
    Id       uint64 `json:"id"`
    AuthorId uint64 `json:"authorId"`
    Edited   bool   `json:"edited"`
}*/
