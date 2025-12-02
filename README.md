# CVScan - Generate insights from batches of CVs
CVScan is a script that allows you to run a large amounts of questions over CVs.

## Features
- Two question types: boolean and free-text
    - Booleans can be yes or no
    - Free-text can be anything you would like, names, summaries, etc...
- Extract multiple question sets across multiple candidates in paralell, with a tunable parameter to maximise speed for your specific rate limits
- Format results into CSV so excel, python, or anything else can read them
- Optionally point at a different LLM provider (the provider must be using OpenAI chat completions API format though)

## Instalation
Run the following command:
- `go install github.com/JoshPattman/cvscan@latest`

## Usage
> Note: At any time you can run cvscan -h to show the help dialog.
1. Create A config file in the directory that you wish to run the script. It should look somthing like this:
```json
{
    "views": {
        "programmer": {
            "pretty_name": "Programmer",
            "score_checklist": {
                "programmer": {
                    "question": "Has the candidate got experience in programming"
                },
                "python": {
                    "question": "Does the candidate know Python",
                    "weight": 2
                }
            },
            "specific_questions": {
                "summary": {
                    "question": "Provide a brief summary of the candidate's programming experience (one sentence)."
                },
                "name": {
                    "question": "What is the candidate's full name? Respond in all caps."
                }
            }
        },
        "finance": {
            "pretty_name": "Finance",
            "score_checklist": {
                "finance_exp": {
                    "question": "Does the candidate have experience in finance?"
                },
                "accounting_exp": {
                    "question": "Does the candidate have experience in accounting?"
                }
            }
        }
    }
}
```

2. Create a folder called `pdf` and put your CVs in it.
3. Run `cvscan -r <number of repeats, if not specified will default to 5> -k <openai key> -u <openai url, if not specified will default to openai chat completions> -m <model name, if not specified default to gpt-4.1>`
    - For example `cvscan -k sk-proj-...`
4. You will get a directory called result, which will contain reports, and a directory called text which contains all the text from the pdfs.