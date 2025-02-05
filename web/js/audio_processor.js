
// AudioWorkletProcessor is an interface that has no process() method but you MUST implement it
class AudioProcessor extends AudioWorkletProcessor {
    constructor() {
        super();
        this.keepProcessing = true;
        this.port.onmessage = (event) => {
            if (event.data.command === 'stop') {
                console.log("STOP Received, closing port")
                this.port.close();
                this.keepProcessing = false;
            }
        };
    }
    // https://developer.mozilla.org/en-US/docs/Web/API/AudioWorkletProcessor/process
    process(inputs, outputs, parameters) {
        const input = inputs[0]; // Get the first input channel
        if (input.length > 0) {
            const channelData = input[0]; // Get the audio data from the first channel
            // Process the audio data here
            console.log(channelData); // Log the raw audio data
        }
        if (!this.keepProcessing) {
            throw new Error("I don't know how to terminate this gracefully");
        }

        return false; // Keep the processor alive. This doesn't even terminate if false is returned

    }
}

registerProcessor("audio_processor", AudioProcessor);