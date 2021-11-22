import { Factory } from 'miragejs';
import { trait } from 'ember-cli-mirage';

export default Factory.extend({
  example: trait({
    user: 'example',
    password: 'example',
    privateKeyPem: btoa('example'),
  }),
});
