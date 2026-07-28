import { extractText, getDocumentProxy } from "unpdf";

export async function pdfToText(buffer: ArrayBuffer): Promise<string> {
  try {
    const pdf = await getDocumentProxy(new Uint8Array(buffer));
    const { text } = await extractText(pdf, { mergePages: true });
    return (Array.isArray(text) ? text.join("\n") : text).trim();
  } catch {
    return "";
  }
}
