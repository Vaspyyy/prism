// Definitions whose value is a template string end with it.
const query = (name: string, fields: ReadonlyArray<string>): string =>
  `query ${name} {\n${fields.map((field) => `  ${field}\n`).join("")}}\n`

const config = {
  schema: "./schema.graphql",
}

export type Route = `/${string}`

export const multi = (rows: string[]) => `
  ${rows.join("\n")}
`

export function after() {
  return config
}
