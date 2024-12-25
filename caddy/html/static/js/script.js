document.addEventListener('DOMContentLoaded', function () {
  console.log("dom ready");
  const switchColor = setInterval(function () {
    var el = document.getElementById("def");
    console.log(el);
    if (el.classList.contains("abc"))
      el.classList.remove("abc");
    else
      el.classList.add("abc");
  }, 1000);

  console.log(switchColor);
});
