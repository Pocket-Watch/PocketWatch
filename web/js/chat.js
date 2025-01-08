import * as api from "./api.js";
import { getById, div, a, span, img, svg, button } from "./util.js";

export { Chat }

class Chat {
    constructor() {
        this.chatTextContent = getById("chat_text_content");
        // etc...
    }
}
