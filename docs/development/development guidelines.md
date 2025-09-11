Development principles:
* Reuse code whenever possible. Ask for help when not possible and propose approaches
* Ensure functionality aligns with business requirements. DO NOT implement code that is not supported or backed up by a requirement
* Integrate all new code with existing code: Any new code that is not integrated with the main code and is not backed up by a requirement should be removed before completing this task.
* Ask for input for critical decissions, do not make assumptions if they are not backed up by existing code or defined as requirements. Provide preference with justification.
* ALWAYS log errors, never ignore them.
* Ensure NO errors are ignored.
* Avoid duplicating structure names. ALWAYS use unique names that align best with the business purpose and can't lead to confusion.
* DO NOT use local type definitions to resolve import cycles. Instead, leverage on shared types or move existing local types to the shared types package and updated references across the code to use them.


Testing principles:
* Reuse test framework code unless not available, in which case ask for guidance and wait for response. Propose suggestions.
* ALWAYS avoid the null-testing anti-pattern
* Use ginkgo/gomega BDD test framework.
* Use existing mocks and fake client for K8s interaction.
* Ensure all tests are backed up by a business requirement.
* Ensure all business requirements have at least a unit test backing them up.
* DO NOT test the implementation of the business requirement, test the actual business requirement expectations.
* Ask for input for critical decissions, do not make assumptions if they are not backed up by existing code or defined as requirements. Provide preference with justification.
* Ensure test assertions are aligned with business requirements.
* Assertions MUST be backed on business outcomes. AVOID weak business validations (not nil, > 0, < 0, not empty, etc...). Whenever string is randomly generated, use substring validation (contains) or numbers (range). Weak assertions don't provide a strong business requirements confidence.
* Prefer to reuse existing mocks and extend them than create local ones. When no mocks exist, create one in the shared directory where all the other mocks reside.
* Ensure all errors are captured and validated correctly.


After completing task:
* Whenever applies, ensure code builds without errors related to changes made during this session. Address any error found based on the previous principles.
* Propose enhancements with a confidence level >=60% based on the current milestone scope. DO NOT implement anything unless otherwise stated.