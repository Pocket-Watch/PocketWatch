import * as api from "./api.js";
import { getById } from "./util.js";

export { Chat }

const CHARACTER_LIMIT = 1000;

class Chat {
    constructor() {
        this.chatInput = getById("chat_input_box");
        this.chatArea = getById("chat_text_content");
        this.sendMessageButton = getById("chat_send_button");

        this.attachListeners();
    }

    clear() {
        while (this.chatArea.lastChild) {
            this.chatArea.removeChild(this.chatArea.lastChild);
        }
    }

    addMessage(chatMsg, allUsers) {
        let chatDiv = document.createElement("div");
        let index = allUsers.findIndex(user => user.id === chatMsg.authorId);
        let user = allUsers[index];

        let username = "Deleted user"
        if (user) {
            username = user.username;
        }

        chatDiv.innerText = "[" + username + "] " + chatMsg.message;
        this.chatArea.appendChild(chatDiv);
        this.chatArea.scrollTo(0, this.chatArea.clientHeight)
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
