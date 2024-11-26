var slider = document.getElementById("slider");
var sliderContainer = document.getElementById("sliderContainer");
var sliderValue = document.getElementById("sliderValue");
sliderValue.style.pointerEvents = "none";

var isDragging = false;

sliderContainer.addEventListener("mousedown", function (e) {
    calculateOffset(e)
    isDragging = true;
})


document.addEventListener("mouseup", function() {
    isDragging = false;
});

document.addEventListener("mousemove", function(e) {
    if(!isDragging) {
        return;
    }

    calculateOffset(e)
});

function calculateOffset(e) {
    let rect = sliderContainer.getBoundingClientRect();
    let offsetX = e.clientX - rect.left;

    // Ensure the slider stays within bounds
    if (offsetX < 0) offsetX = 0;
    if (offsetX > rect.width) offsetX = rect.width;

    slider.style.left = offsetX + 'px';

    // Calculate the slider value as a percentage
    let percentage = Math.round((offsetX / rect.width) * 100);
    sliderValue.innerHTML = percentage + "%";
}

