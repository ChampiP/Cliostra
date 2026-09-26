const schemaBuilder = () => ({
  describe: () => schemaBuilder(),
  optional: () => schemaBuilder(),
})

export const tool = (def) => def
tool.schema = {
  enum: () => schemaBuilder(),
  string: () => schemaBuilder(),
  boolean: () => schemaBuilder(),
}

export default { tool }
