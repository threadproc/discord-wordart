$(document).ready(function () {
  console.log("ready, centering element");

  var wacont = $("div#wordart.wordart-container");
  var wa = $("div#wordart .wordart");

  console.log(wa[0]);

  var bbox = wa[0].getBoundingClientRect();
  console.log(bbox);

  wacont.width(bbox.width);
  wacont.height(bbox.height);
  wacont.data("height", bbox.height + 40);

  var topOffset = 0 - bbox.top + 20;
  var totalHeight = bbox.height + 40;
  if (totalHeight < 600) {
    topOffset += (600 - totalHeight) / 2;
  }

  console.log(topOffset);
  wa.css("top", topOffset + "px");
});