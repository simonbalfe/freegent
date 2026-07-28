package apify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/simonbalfe/freegent/internal/agent"
)

type Tool = agent.Tool
type ToolResult = agent.ToolResult
type FetchAttempt = agent.FetchAttempt

type apifyTool struct {
	name        string
	description string
	schema      map[string]any
	actor       func() string
	input       func(map[string]any) (map[string]any, error)
	output      func([]map[string]any, map[string]any) (any, []string)
}

func (t apifyTool) Name() string           { return t.name }
func (t apifyTool) Description() string    { return t.description }
func (t apifyTool) Schema() map[string]any { return t.schema }

func (t apifyTool) Run(ctx context.Context, input map[string]any) (ToolResult, error) {
	actorInput, err := t.input(input)
	if err != nil {
		return ToolResult{}, err
	}
	actor := t.actor()
	result, err := runApifyActor(ctx, actor, actorInput)
	if err != nil {
		return ToolResult{}, err
	}
	output, urls := t.output(result.Items, input)
	encoded, err := json.Marshal(output)
	if err != nil {
		return ToolResult{}, err
	}
	return ToolResult{
		Text:     bounded(string(encoded)),
		URLs:     urls,
		SeenURLs: urls,
		Provider: "apify:" + actor,
		Attempts: []FetchAttempt{{Provider: "apify:" + actor, Outcome: strings.ToLower(result.Status), DurationMS: result.DurationMS, Detail: fmt.Sprintf("run %s, dataset %s, %d items", result.RunID, result.DatasetID, len(result.Items))}},
	}, nil
}

func Tools() []Tool {
	return []Tool{
		linkedinProfileTool(),
		linkedinPostsTool(),
		linkedinReactionsTool(),
		linkedinPeopleTool(),
		linkedinCompanyTool(),
		crunchbaseCompanyTool(),
	}
}

func linkedinProfileTool() Tool {
	return apifyTool{
		name:        "linkedin_profile",
		description: "Get a person's LinkedIn profile as structured data: name, headline, location, about, experience, follower count, and connection count. Use this instead of fetching LinkedIn pages. The URL must come from the input row or gathered evidence. Costs credits, so call once per person.",
		schema:      objectSchema(map[string]any{"url": map[string]any{"type": "string", "format": "uri"}}, []string{"url"}),
		actor:       func() string { return envOr("APIFY_LINKEDIN_PROFILE_ACTOR", "harvestapi~linkedin-profile-scraper") },
		input: func(input map[string]any) (map[string]any, error) {
			rawURL := strings.TrimSpace(stringValue(input["url"]))
			if rawURL == "" {
				return nil, errors.New("linkedin_profile requires url")
			}
			return map[string]any{"url": rawURL}, nil
		},
		output: func(items []map[string]any, input map[string]any) (any, []string) {
			if len(items) == 0 {
				return map[string]any{"profile": nil}, nil
			}
			item := items[0]
			experienceItems := objectSlice(item["experience"])
			experiences := []any{}
			for _, experience := range experienceItems[:min(5, len(experienceItems))] {
				experiences = append(experiences, map[string]any{
					"position": text(experience["position"]),
					"company":  text(experience["companyName"]),
					"duration": text(experience["duration"]),
				})
			}
			rawURL := firstNonEmpty(text(item["linkedinUrl"]), stringValue(input["url"]))
			profile := map[string]any{
				"name":             strings.TrimSpace(text(item["firstName"]) + " " + text(item["lastName"])),
				"headline":         text(item["headline"]),
				"location":         text(object(item["location"])["linkedinText"]),
				"about":            clipLength(text(item["about"]), 1500),
				"linkedinUrl":      rawURL,
				"publicIdentifier": text(item["publicIdentifier"]),
				"followers":        nullableNumber(item["followerCount"]),
				"connections":      nullableNumber(item["connectionsCount"]),
				"experience":       experiences,
			}
			return map[string]any{"profile": profile}, validURLs(rawURL)
		},
	}
}

func linkedinPostsTool() Tool {
	return apifyTool{
		name:        "linkedin_posts",
		description: "Get recent LinkedIn posts for a person or company: text, date, engagement counts, and post URLs. The profile URL must come from the input row or gathered evidence. Costs credits, so keep maxPosts small.",
		schema: objectSchema(map[string]any{
			"profileUrl": map[string]any{"type": "string", "format": "uri"},
			"maxPosts":   map[string]any{"type": "integer", "minimum": 1, "maximum": 20, "default": 5},
		}, []string{"profileUrl"}),
		actor: func() string { return envOr("APIFY_LINKEDIN_POSTS_ACTOR", "harvestapi~linkedin-profile-posts") },
		input: func(input map[string]any) (map[string]any, error) {
			profileURL := strings.TrimSpace(stringValue(input["profileUrl"]))
			if profileURL == "" {
				return nil, errors.New("linkedin_posts requires profileUrl")
			}
			return map[string]any{"targetUrls": []string{profileURL}, "maxPosts": boundedInt(input["maxPosts"], 5, 1, 20)}, nil
		},
		output: func(items []map[string]any, input map[string]any) (any, []string) {
			limit := boundedInt(input["maxPosts"], 5, 1, 20)
			posts := []any{}
			urls := []string{}
			for _, item := range items[:min(limit, len(items))] {
				postURL := text(item["linkedinUrl"])
				postedAt := object(item["postedAt"])
				engagement := object(item["engagement"])
				posts = append(posts, map[string]any{
					"url":      postURL,
					"postedAt": firstNonEmpty(text(postedAt["date"]), text(postedAt["postedAgoText"])),
					"text":     clipLength(text(item["content"]), 600),
					"likes":    numberOrZero(engagement["likes"]),
					"comments": numberOrZero(engagement["comments"]),
					"shares":   numberOrZero(engagement["shares"]),
				})
				urls = append(urls, validURLs(postURL)...)
			}
			return map[string]any{"posts": posts}, uniqueStrings(urls)
		},
	}
}

func linkedinReactionsTool() Tool {
	return apifyTool{
		name:        "linkedin_post_reactions",
		description: "Get people who reacted to a LinkedIn post, including reaction type, name, position, and profile URL. The post URL must come from the input row or gathered evidence. Costs credits, so keep maxReactions small.",
		schema: objectSchema(map[string]any{
			"postUrl":      map[string]any{"type": "string", "format": "uri"},
			"maxReactions": map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 20},
		}, []string{"postUrl"}),
		actor: func() string { return envOr("APIFY_LINKEDIN_REACTIONS_ACTOR", "harvestapi~linkedin-post-reactions") },
		input: func(input map[string]any) (map[string]any, error) {
			postURL := strings.TrimSpace(stringValue(input["postUrl"]))
			if postURL == "" {
				return nil, errors.New("linkedin_post_reactions requires postUrl")
			}
			return map[string]any{"posts": []string{postURL}, "maxItems": boundedInt(input["maxReactions"], 20, 1, 100)}, nil
		},
		output: func(items []map[string]any, input map[string]any) (any, []string) {
			limit := boundedInt(input["maxReactions"], 20, 1, 100)
			reactions := []any{}
			for _, item := range items[:min(limit, len(items))] {
				actor := object(item["actor"])
				reactions = append(reactions, map[string]any{
					"type":        text(item["reactionType"]),
					"name":        text(actor["name"]),
					"position":    clipLength(text(actor["position"]), 120),
					"linkedinUrl": text(actor["linkedinUrl"]),
				})
			}
			return map[string]any{"reactions": reactions}, nil
		},
	}
}

func linkedinPeopleTool() Tool {
	return apifyTool{
		name:        "linkedin_find_people",
		description: "Find people at a company through LinkedIn employee search, filtered by title or query. Returns name, title, location, profile URL, and optionally work email. Pass an exact company name or a verified LinkedIn company URL. Costs credits, especially with findEmails.",
		schema: objectSchema(map[string]any{
			"company":     map[string]any{"type": "string"},
			"jobTitles":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "maxItems": 8},
			"searchQuery": map[string]any{"type": "string"},
			"maxItems":    map[string]any{"type": "integer", "minimum": 1, "maximum": 10, "default": 5},
			"findEmails":  map[string]any{"type": "boolean", "default": false},
		}, []string{"company"}),
		actor: func() string { return envOr("APIFY_LINKEDIN_EMPLOYEES_ACTOR", "harvestapi~linkedin-company-employees") },
		input: func(input map[string]any) (map[string]any, error) {
			company := strings.TrimSpace(stringValue(input["company"]))
			if company == "" {
				return nil, errors.New("linkedin_find_people requires company")
			}
			actorInput := map[string]any{
				"companies":          []string{company},
				"maxItems":           boundedInt(input["maxItems"], 5, 1, 10),
				"profileScraperMode": "Short ($4 per 1k)",
			}
			if titles := stringSlice(input["jobTitles"]); len(titles) > 0 {
				actorInput["jobTitles"] = titles[:min(8, len(titles))]
			}
			if query := strings.TrimSpace(stringValue(input["searchQuery"])); query != "" {
				actorInput["searchQuery"] = query
			}
			if boolValue(input["findEmails"]) {
				actorInput["profileScraperMode"] = "Full + email search ($12 per 1k)"
			}
			return actorInput, nil
		},
		output: func(items []map[string]any, input map[string]any) (any, []string) {
			limit := boundedInt(input["maxItems"], 5, 1, 10)
			people := []any{}
			urls := []string{}
			for _, item := range items[:min(limit, len(items))] {
				current := firstCurrentPosition(objectSlice(item["currentPositions"]))
				linkedinURL := text(item["linkedinUrl"])
				people = append(people, map[string]any{
					"name":        firstNonEmpty(strings.TrimSpace(text(item["firstName"])+" "+text(item["lastName"])), text(item["name"])),
					"title":       firstNonEmpty(text(current["title"]), text(item["position"]), text(item["headline"])),
					"company":     text(current["companyName"]),
					"location":    text(object(item["location"])["linkedinText"]),
					"linkedinUrl": linkedinURL,
					"email":       emailValue(item),
				})
				urls = append(urls, validURLs(linkedinURL)...)
			}
			return map[string]any{"people": people}, uniqueStrings(urls)
		},
	}
}

func linkedinCompanyTool() Tool {
	return apifyTool{
		name:        "linkedin_company",
		description: "Get a company's LinkedIn firmographics: exact employee count, size range, industry, founded year, headquarters, followers, website, and description. Pass the exact company name and let the actor resolve it. A URL is accepted only when it came from the input row or gathered evidence. Costs credits, so call once per company.",
		schema:      objectSchema(map[string]any{"company": map[string]any{"type": "string"}}, []string{"company"}),
		actor:       func() string { return envOr("APIFY_LINKEDIN_COMPANY_ACTOR", "harvestapi~linkedin-company") },
		input: func(input map[string]any) (map[string]any, error) {
			company := strings.TrimSpace(stringValue(input["company"]))
			if company == "" {
				return nil, errors.New("linkedin_company requires company")
			}
			if isHTTPURL(company) {
				return map[string]any{"companies": []string{company}}, nil
			}
			return map[string]any{"searches": []string{company}}, nil
		},
		output: func(items []map[string]any, input map[string]any) (any, []string) {
			if len(items) == 0 {
				return map[string]any{"company": nil}, nil
			}
			item := items[0]
			locations := objectSlice(item["locations"])
			headquarters := map[string]any{}
			for _, location := range locations {
				if boolValue(location["headquarter"]) {
					headquarters = location
					break
				}
			}
			if len(headquarters) == 0 && len(locations) > 0 {
				headquarters = locations[0]
			}
			ranges := object(item["employeeCountRange"])
			rangeValue := any(nil)
			if start := nullableNumber(ranges["start"]); start != nil {
				if end := nullableNumber(ranges["end"]); end != nil {
					rangeValue = fmt.Sprintf("%v-%v", start, end)
				} else {
					rangeValue = fmt.Sprintf("%v+", start)
				}
			}
			industry := ""
			if industries := objectSlice(item["industries"]); len(industries) > 0 {
				industry = firstNonEmpty(text(industries[0]["title"]), text(industries[0]["name"]))
			}
			linkedinURL := text(item["linkedinUrl"])
			if linkedinURL == "" && isHTTPURL(stringValue(input["company"])) {
				linkedinURL = stringValue(input["company"])
			}
			company := map[string]any{
				"name":               text(item["name"]),
				"linkedinUrl":        linkedinURL,
				"website":            text(item["website"]),
				"tagline":            text(item["tagline"]),
				"description":        clipLength(text(item["description"]), 1000),
				"employeeCount":      nullableNumber(item["employeeCount"]),
				"employeeCountRange": rangeValue,
				"followers":          nullableNumber(item["followerCount"]),
				"foundedYear":        foundedYear(item["foundedOn"]),
				"industry":           industry,
				"specialities":       limitStrings(stringSlice(item["specialities"]), 10),
				"headquarters":       joinNonEmpty(text(headquarters["city"]), text(headquarters["geographicArea"]), text(headquarters["country"])),
			}
			return map[string]any{"company": company}, validURLs(linkedinURL)
		},
	}
}

func crunchbaseCompanyTool() Tool {
	return apifyTool{
		name:        "crunchbase_company",
		description: "Fallback only. Get Crunchbase funding and firmographics including total funding, latest round, investors, founders, employee range, headquarters, founded year, and IPO status. Pass a verified Crunchbase organization URL or the exact company name. Costs credits, so call at most once.",
		schema:      objectSchema(map[string]any{"company": map[string]any{"type": "string"}}, []string{"company"}),
		actor:       func() string { return envOr("CRUNCHBASE_ACTOR", "parseforge~crunchbase-scraper") },
		input: func(input map[string]any) (map[string]any, error) {
			company := strings.TrimSpace(stringValue(input["company"]))
			if company == "" {
				return nil, errors.New("crunchbase_company requires company")
			}
			if isHTTPURL(company) {
				return map[string]any{"startUrls": []map[string]any{{"url": company}}, "maxItems": 1}, nil
			}
			return map[string]any{"searchQuery": company, "maxItems": 1}, nil
		},
		output: func(items []map[string]any, input map[string]any) (any, []string) {
			if len(items) == 0 {
				return map[string]any{"company": nil}, nil
			}
			item := items[0]
			crunchbaseURL := firstNonEmpty(text(item["cbUrl"]), text(item["crunchbaseUrl"]), text(item["url"]))
			if crunchbaseURL == "" && isHTTPURL(stringValue(input["company"])) {
				crunchbaseURL = stringValue(input["company"])
			}
			company := map[string]any{
				"name":            text(item["name"]),
				"crunchbaseUrl":   crunchbaseURL,
				"website":         text(item["website"]),
				"foundedYear":     firstValue(item["founded"], item["foundedOn"]),
				"employeeCount":   nullableValue(item["employeeCount"]),
				"industry":        firstString(item["industries"]),
				"headquarters":    firstNonEmpty(text(item["headquarters"]), joinNonEmpty(text(item["city"]), text(item["region"]), text(item["country"]))),
				"totalFundingUsd": firstValue(item["totalFundingUsd"], item["totalFunding"]),
				"lastRound": map[string]any{
					"type":      firstNonEmpty(text(item["lastRoundType"]), text(item["lastFundingType"])),
					"amountUsd": firstValue(item["lastRoundAmountUsd"], item["lastFundingAmountUsd"]),
					"date":      firstNonEmpty(text(item["lastRoundDate"]), text(item["lastFundingOn"])),
				},
				"founders":      entityNames(item["founders"], 6),
				"leadInvestors": entityNames(firstValue(item["leadInvestors"], item["investors"]), 10),
				"ipoStatus":     firstNonEmpty(text(item["ipoStatus"]), text(item["operatingStatus"])),
			}
			return map[string]any{"company": company}, validURLs(crunchbaseURL)
		},
	}
}
