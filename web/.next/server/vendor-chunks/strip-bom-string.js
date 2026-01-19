"use strict";
/*
 * ATTENTION: An "eval-source-map" devtool has been used.
 * This devtool is neither made for production nor for readable output files.
 * It uses "eval()" calls to create a separate source file with attached SourceMaps in the browser devtools.
 * If you are trying to read the output file, select a different devtool (https://webpack.js.org/configuration/devtool/)
 * or disable the default devtool with "devtool: false".
 * If you are looking for production-ready output files, see mode: "production" (https://webpack.js.org/configuration/mode/).
 */
exports.id = "vendor-chunks/strip-bom-string";
exports.ids = ["vendor-chunks/strip-bom-string"];
exports.modules = {

/***/ "(rsc)/./node_modules/strip-bom-string/index.js":
/*!************************************************!*\
  !*** ./node_modules/strip-bom-string/index.js ***!
  \************************************************/
/***/ ((module) => {

eval("/*!\n * strip-bom-string <https://github.com/jonschlinkert/strip-bom-string>\n *\n * Copyright (c) 2015, 2017, Jon Schlinkert.\n * Released under the MIT License.\n */\n\n\n\nmodule.exports = function(str) {\n  if (typeof str === 'string' && str.charAt(0) === '\\ufeff') {\n    return str.slice(1);\n  }\n  return str;\n};\n//# sourceURL=[module]\n//# sourceMappingURL=data:application/json;charset=utf-8;base64,eyJ2ZXJzaW9uIjozLCJmaWxlIjoiKHJzYykvLi9ub2RlX21vZHVsZXMvc3RyaXAtYm9tLXN0cmluZy9pbmRleC5qcyIsIm1hcHBpbmdzIjoiQUFBQTtBQUNBO0FBQ0E7QUFDQTtBQUNBO0FBQ0E7O0FBRWE7O0FBRWI7QUFDQTtBQUNBO0FBQ0E7QUFDQTtBQUNBIiwic291cmNlcyI6WyJ3ZWJwYWNrOi8vY2tiLXdlYi8uL25vZGVfbW9kdWxlcy9zdHJpcC1ib20tc3RyaW5nL2luZGV4LmpzPzc3ODIiXSwic291cmNlc0NvbnRlbnQiOlsiLyohXG4gKiBzdHJpcC1ib20tc3RyaW5nIDxodHRwczovL2dpdGh1Yi5jb20vam9uc2NobGlua2VydC9zdHJpcC1ib20tc3RyaW5nPlxuICpcbiAqIENvcHlyaWdodCAoYykgMjAxNSwgMjAxNywgSm9uIFNjaGxpbmtlcnQuXG4gKiBSZWxlYXNlZCB1bmRlciB0aGUgTUlUIExpY2Vuc2UuXG4gKi9cblxuJ3VzZSBzdHJpY3QnO1xuXG5tb2R1bGUuZXhwb3J0cyA9IGZ1bmN0aW9uKHN0cikge1xuICBpZiAodHlwZW9mIHN0ciA9PT0gJ3N0cmluZycgJiYgc3RyLmNoYXJBdCgwKSA9PT0gJ1xcdWZlZmYnKSB7XG4gICAgcmV0dXJuIHN0ci5zbGljZSgxKTtcbiAgfVxuICByZXR1cm4gc3RyO1xufTtcbiJdLCJuYW1lcyI6W10sInNvdXJjZVJvb3QiOiIifQ==\n//# sourceURL=webpack-internal:///(rsc)/./node_modules/strip-bom-string/index.js\n");

/***/ })

};
;