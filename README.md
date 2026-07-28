# Freegent

Freegent is a general-purpose AI research agent for spreadsheets, similar to a self-hosted Clay research column.

Give it any CSV and a prompt. Freegent researches every row and returns the same CSV with one new `answer` column.

## Example

Start with:

```csv
subject,type,url
Figma,company,https://figma.com
Dario Amodei,person,
EU AI Act,topic,
```

Run:

```bash
freegent \
  --csv research.csv \
  --instructions "Use current primary sources. Do not guess unsupported facts." \
  --prompt "Research {{subject}}, which is a {{type}}. Use {{url}} when supplied. Return a concise factual brief." \
  > results.csv
```

You get:

```csv
subject,type,url,answer
Figma,company,https://figma.com,...
Dario Amodei,person,,...
EU AI Act,topic,,...
```

Use Freegent for:

- account research
- people and role research
- market and competitor research
- product comparisons
- news and buying signals
- URL extraction and summarization
- row classification, scoring, and verification
- lead qualification and sales personalization

## Set up Freegent

You need:

1. [Docker](https://docs.docker.com/get-docker/)
2. an [OpenRouter API key](https://openrouter.ai/settings/keys)
3. a search key from [Serper](https://serper.dev/), [Exa](https://dashboard.exa.ai/api-keys), or [Tavily](https://app.tavily.com/home)

Run the installer:

```bash
curl -fsSL https://raw.githubusercontent.com/simonbalfe/freegent/main/install.sh | bash
```

Enter your keys when asked. The installer starts Freegent and installs the `freegent` command.

Check that it worked:

```bash
freegent --help
```

Open the dashboard:

[http://localhost:8080/dashboard](http://localhost:8080/dashboard)

## Research a CSV

```bash
freegent \
  --csv research.csv \
  --instructions "Use current evidence and clearly state uncertainty." \
  --prompt "Research {{subject}} and answer using current evidence." \
  > results.csv
```

Any CSV header can be used in the prompt with `{{field_name}}`.

Each row is a separate research task. Your rows can contain companies, people, products, URLs, markets, topics, or anything else the agent can research.
`--instructions` applies the same research rules to the whole job. `--prompt` is rendered separately for each row using its CSV values.

## Research one row

```bash
freegent \
  --row '{"subject":"EU AI Act","question":"What changed most recently?"}' \
  --instructions "Prefer official EU sources and identify the effective date." \
  --prompt "Research {{subject}} and answer: {{question}}"
```

## Request a structured answer

Freegent returns a text answer by default. Use `--schema` when every answer needs the same fields:

```bash
freegent \
  --csv research.csv \
  --prompt "Research {{subject}} and classify the result." \
  --schema '{"summary":"string","category":"string","source":"string"}' \
  > structured-results.csv
```

The structured result is stored inside the single `answer` column.

## Run a large list

Use `--detach` to let a job continue without keeping the terminal open:

```bash
freegent \
  --csv research.csv \
  --prompt "Research {{subject}}." \
  --detach
```

Freegent returns the job ID, dashboard link, and download link.

## Manage Freegent

Check that everything is running:

```bash
cd ~/freegent
docker compose ps
```

View logs:

```bash
docker compose logs -f api worker openextract
```

Update:

```bash
cd ~/freegent
git pull --ff-only
docker compose pull
docker compose up -d --force-recreate
```

Jobs and results are saved locally and survive restarts.
