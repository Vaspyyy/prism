import React from "react";

export function View(props: { name: string }) {
  const css = `
    function notAComponent() {
  `;
  return <div className={css}>{props.name}</div>;
}

export const Item = ({ label }: { label: string }) => {
  return <li>{label}</li>;
};
