import * as api from "./api.js";
import { getById, div, a, span, img, svg, button } from "./util.js";

export { Chat }

const CHARACTER_LIMIT = 500;
class Chat {
    constructor() {
        this.chatInput = getById("chat_input_box");
        this.chatArea = getById("chat_text_content");
        this.sendMessageButton = getById("chat_send_button");

        this.attachListeners();

    }

    addMessage(chatMsg, allUsers) {
        let chatDiv = document.createElement("div");
        let index = allUsers.findIndex(user => user.id === chatMsg.authorId);
        let userName = allUsers[index].username;

        chatDiv.innerText = "[" + userName + "] " + chatMsg.message;
        this.chatArea.appendChild(chatDiv);
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
        if (this.chatInput.innerText > CHARACTER_LIMIT) {
            // This is handled server side for length
            return;
        }
        api.apiChatSend(this.chatInput.value)
        this.chatInput.value = "";
    }
}
