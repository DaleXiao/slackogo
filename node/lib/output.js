import chalk from 'chalk';

export class Printer {
  constructor(format = 'human') {
    this.format = format; // 'human' | 'plain' | 'json'
  }

  json(data) {
    console.log(JSON.stringify(data, null, 2));
  }

  plain(...fields) {
    console.log(fields.join('\t'));
  }

  human(msg) {
    console.log(msg);
  }

  header(msg) {
    if (this.format === 'human') console.log(chalk.cyan.bold(msg));
  }

  success(msg) {
    if (this.format === 'human') console.log(chalk.green(msg));
  }

  error(msg) {
    console.error(chalk.red(msg));
  }

  auto(jsonData, plainRows, humanFn) {
    switch (this.format) {
      case 'json':
        this.json(jsonData);
        break;
      case 'plain':
        for (const row of plainRows) this.plain(...row);
        break;
      default:
        humanFn();
    }
  }
}
