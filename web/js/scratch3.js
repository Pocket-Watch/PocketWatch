var slider = document.getElementById("slider");
var sliderContainer = document.getElementById("sliderContainer");
var sliderValue = document.getElementById("sliderValue");
sliderValue.style.pointerEvents = "none";

var isDragging = false;

console.log("Running!");

function cursorPress(e) {
    isDragging = true;
    calculateOffset(e)
}

function cursorRelease() {
    isDragging = false;
}

function cursorMove(e) {
    if(!isDragging) {
        return;
    }
    calculateOffset(e)
}

sliderContainer.addEventListener("touchstart", cursorPress);
document.addEventListener("touchmove", cursorMove);
sliderContainer.addEventListener("touchend", cursorRelease);

sliderContainer.addEventListener("mousedown", cursorPress);
document.addEventListener("mousemove", cursorMove);
document.addEventListener("mouseup", cursorRelease);

function calculateOffset(e) {
    let rect = sliderContainer.getBoundingClientRect();
    let offsetX;

    if (e.touches) {
        offsetX = e.touches[0].clientX - rect.left;
    } else {
        offsetX = e.clientX - rect.left;
    }

    // Ensure the touch doesn't exceed slider bounds
    if (offsetX < 0) offsetX = 0;
    if (offsetX > rect.width) offsetX = rect.width;

    slider.style.left = offsetX + 'px';

    // Calculate the slider value as a percentage
    let percentage = Math.round((offsetX / rect.width) * 100);
    sliderValue.innerHTML = percentage + "%";
}
