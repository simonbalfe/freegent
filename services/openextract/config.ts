import { z } from "zod";

const environmentInteger = (fallback: number, minimum: number) =>
  z.preprocess(
    (value) => (value === undefined || value === "" ? fallback : value),
    z.coerce.number().int().min(minimum),
  );

const environmentSchema = z
  .object({
    PORT: environmentInteger(8081, 1).pipe(z.number().max(65535)),
    OPENEXTRACT_MAX_CONCURRENCY: environmentInteger(20, 1),
    OPENEXTRACT_BROWSER_CONCURRENCY: environmentInteger(4, 1),
    OPENEXTRACT_MAX_WAITING: environmentInteger(100, 0),
  })
  .superRefine((value, context) => {
    if (value.OPENEXTRACT_BROWSER_CONCURRENCY > value.OPENEXTRACT_MAX_CONCURRENCY) {
      context.addIssue({
        code: "custom",
        message: "cannot exceed OPENEXTRACT_MAX_CONCURRENCY",
        path: ["OPENEXTRACT_BROWSER_CONCURRENCY"],
      });
    }
  });

export type OpenExtractEnvironment = z.infer<typeof environmentSchema>;

export function parseEnvironment(input: Record<string, string | undefined>): OpenExtractEnvironment {
  return environmentSchema.parse(input);
}
