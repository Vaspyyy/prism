// Shapes bundlers emit.
/**
* @license
* SPDX-License-Identifier: BSD-3-Clause
*/ function licensed(r) {
	return r;
}

const prefixed = (t) => names.includes(t) ? "__" + t : t, options = {
	textNodeName: "#text",
	tagValueProcessor: function(t, e) {
		return e;
	},
	trimValues: true
};

function after() {}

const view = html`
  <input @input=${(e) => update(e.target.value)}>
  ${when((el) => {
		el.saveSettings = () => {
			return true;
		};
	})}
`;

const inject = `(${String(() => {
	window.saveSettings = () => {
		return true;
	};
})})()`;
