# Freegent

Freegent researches a list of companies for you.

Give it a CSV and a question. It researches every row and returns the same CSV with one new `answer` column.

## Example

Start with:

```csv
company,domain
Figma,figma.com
Linear,linear.app
Notion,notion.com
```

Run:

```bash
freegent \
  --csv accounts.csv \
  --prompt "Research {{company}} at {{domain}}. Explain what they sell and who they sell to." \
  > researched-accounts.csv
```

You get:

```csv
company,domain,answer
Figma,figma.com,...
Linear,linear.app,...
Notion,notion.com,...
```

Use Freegent for:

- account research
- lead qualification
- ICP scoring
- buying signals
- sales personalization
- competitor research

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
  --csv accounts.csv \
  --prompt "Is {{company}} a good sales prospect? Give a clear reason." \
  > results.csv
```

Any CSV header can be used in the prompt with `{{field_name}}`.

## Research one company

```bash
freegent \
  --row '{"company":"Figma","domain":"figma.com"}' \
  --prompt "Research {{company}} at {{domain}} and explain what they sell."
```

## Request a structured answer

Freegent returns a text answer by default. Use `--schema` when every answer needs the same fields:

```bash
freegent \
  --csv accounts.csv \
  --prompt "Assess {{company}} as a sales prospect." \
  --schema '{"fit":"high|medium|low","reason":"string","source":"string"}' \
  > scored-accounts.csv
```

The structured result is stored inside the single `answer` column.

## Run a large list

Use `--detach` to let a job continue without keeping the terminal open:

```bash
freegent \
  --csv accounts.csv \
  --prompt "Research {{company}} at {{domain}}." \
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
