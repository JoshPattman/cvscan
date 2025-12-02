# CVScan - Generate insights from batches of CVs
CVScan is a script that allows you to run a large amounts of questions over CVs.


## Instalation
Run the following command:
- `go install github.com/JoshPattman/cvscan@latest`

## Usage
1. Create A config file in the directory that you wish to run the script. It should look somthing like this:
```json
{
    "views": {
        "rogrammer": {
            "pretty_name": "Programmer",
            "score_checklist": {
                "programmer": {
                    "question": "Has the candidate got experience in programming"
                },
                "python": {
                    "question": "Does the candidate know Python",
                    "weight": 2
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
        },
    }
}
```

2. Create a folder called `pdf` and put your CVs in it.
3. Run `cvscan -r <number of repeats, if not specified will default to 5> -k <openai key> -u <openai url, if not specified will default to openai chat completions>`
    - For example `cvscan -k sk-proj-...`
4. You will get a directory called result, which will contain reports, and a directory called text which contains all the text from the pdfs.