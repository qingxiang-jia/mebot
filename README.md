# mebot

Mebot is a commandline tool that automates my daily workflow. It probably won't help you but I share it here just in case. Ideally, this kind of work is done with a UI tool, but Go doesn't have a good native UI solution. So this tool, while having no UI, has some "UI feelings". See below use cases.

## Daily Reading

### Wall Street Journal

```bash
> mebot wsj

- HTML files will be processed and a new.md file will be generated containing the same content. HTML files will be moved into a folder called deleted.

- A file named YYYY-MM-DD WSJ.md will be updated with content from new.md. YYYY-MM-DD is the date of the coming Saturday.

- A file named summary.md will be processed and moved to deleted folder.

- All files, when moved into the deleted folder, will be renamed if there's a naming conflict.
```

### The Economist

Similar process, except the command is: `mebot economist`.
